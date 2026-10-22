package masterdata

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"regexp"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/vertesan/solis-server/internal/db/store/opmaster/version"
	"github.com/vertesan/solis-server/internal/db/store/opsolis"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/utils/config"
	"github.com/vertesan/solis-server/internal/utils/crypto"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

var ErrPublicationBusy = errors.New("a master publication is already running")
var ErrStaleMaster = errors.New("master version has changed; export again")
var hashName = regexp.MustCompile(`^[a-f0-9]{64}$`)

// Coordinator is single-instance. The Mongo job is durable; the gate protects
// this process's gameplay and admin readers, not other running server instances.
type Coordinator struct {
	Gate       Gate
	mu         sync.Mutex
	busy       bool
	unresolved bool
	ctx        context.Context
	wg         sync.WaitGroup
}

var Default = &Coordinator{}

func (c *Coordinator) Start(ctx context.Context) { c.mu.Lock(); c.ctx = ctx; c.mu.Unlock() }
func (c *Coordinator) Wait()                     { c.wg.Wait() }

func (c *Coordinator) Submit(ctx context.Context, request ImportRequest) (*ImportJob, error) {
	if err := validateRequest(request); err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	// Retrying an accepted HTTP submission is safe and returns its existing job.
	old, err := opsolis.FindMasterImportJob(ctx, request.ID)
	if err == nil {
		if !reflect.DeepEqual(old.Request, request) {
			return nil, errors.New("job ID is already used by a different import")
		}
		return old, nil
	}
	if !errors.Is(err, mongo.ErrNoDocuments) {
		return nil, err
	}
	if c.busy || c.unresolved {
		return nil, ErrPublicationBusy
	}
	if c.ctx == nil || c.ctx.Err() != nil {
		return nil, errors.New("master publisher is not running")
	}
	job := newImportJob(request)
	if err := opsolis.InsertMasterImportJob(ctx, job); err != nil {
		return nil, err
	}
	c.busy = true
	c.wg.Add(1)
	work := *job
	go func() {
		defer c.wg.Done()
		defer func() { c.mu.Lock(); c.busy = false; c.mu.Unlock() }()
		// A finite, server-owned deadline survives the CLI disconnecting.
		jobCtx, cancel := context.WithTimeout(c.ctx, 30*time.Minute)
		defer cancel()
		if err := c.run(jobCtx, &work); err != nil {
			zap.L().Error("master publication failed", zap.String("job", work.ID), zap.Error(err))
		}
	}()
	return job, nil
}

func newImportJob(request ImportRequest) *ImportJob {
	h := sha256.Sum256([]byte("solis-master:" + request.ID))
	now := time.Now().UnixMilli()
	return &ImportJob{ID: request.ID, Request: request, Version: hex.EncodeToString(h[:]), State: "queued", CreatedAt: now, UpdatedAt: now}
}

func validateRequest(r ImportRequest) error {
	parsed, err := uuid.Parse(r.ID)
	if err != nil || parsed.String() != r.ID {
		return errors.New("invalid import job ID")
	}
	if r.ExpectedVersion == "" || len(r.Packs) == 0 || len(r.Packs) > 1024 {
		return errors.New("expectedVersion and a bounded nonempty pack list are required")
	}
	seen := make(map[string]bool, len(r.Packs))
	for _, p := range r.Packs {
		if p.Type == "" || seen[p.Type] || !hashName.MatchString(p.BaseFileName) || !hashName.MatchString(p.FileName) || (!r.Reset && p.FileName == p.BaseFileName) || p.FileSize <= 0 || p.FileSize > math.MaxInt32 || p.RowCount < 0 {
			return fmt.Errorf("invalid or repeated staged pack %q", p.Type)
		}
		seen[p.Type] = true
	}
	return nil
}

func (c *Coordinator) run(ctx context.Context, job *ImportJob) (retErr error) {
	paused := false
	// If commit cannot be resolved, remain in maintenance. Startup recovery will
	// resolve it before any clients are served; never resume with a stale snapshot.
	resolved := true
	defer func() {
		if paused && resolved {
			c.Gate.Resume()
		}
		if !resolved {
			c.mu.Lock()
			c.unresolved = true
			c.mu.Unlock()
		}
	}()
	fail := func(cause error) error {
		recoveryCtx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		settled, err := opsolis.FenceFailedMasterImport(recoveryCtx, job.ID, cause.Error(), time.Now().UnixMilli())
		if err != nil {
			resolved = false
			return errors.Join(cause, fmt.Errorf("publication outcome unresolved: %w", err))
		}
		*job = *settled
		if settled.Committed {
			if err := RefreshSnapshot(recoveryCtx); err != nil {
				resolved = false
				return err
			}
			return c.cleanup(recoveryCtx, job)
		}
		// New orphan files can be removed only after abort has been fenced.
		if err := cleanupUncommitted(recoveryCtx, job); err != nil {
			job.State = "abort_cleanup_pending"
			job.Error = errors.Join(cause, err).Error()
			_ = opsolis.UpdateMasterImportJob(recoveryCtx, job)
			return errors.Join(cause, err)
		}
		job.State = "failed"
		job.UpdatedAt = time.Now().UnixMilli()
		return errors.Join(cause, opsolis.UpdateMasterImportJob(recoveryCtx, job))
	}
	job.State = "preparing"
	job.UpdatedAt = time.Now().UnixMilli()
	if err := opsolis.UpdateMasterImportJob(ctx, job); err != nil {
		return fail(err)
	}
	packs, err := preparePublication(ctx, job)
	if err != nil {
		return fail(err)
	}
	paused = true
	if err := c.Gate.Pause(ctx); err != nil {
		return fail(err)
	}
	if err := checkVersion(ctx, job.Request.ExpectedVersion); err != nil {
		return fail(err)
	}
	job.State = "publishing"
	job.UpdatedAt = time.Now().UnixMilli()
	if err := opsolis.UpdateMasterImportJob(ctx, job); err != nil {
		return fail(err)
	}
	if !job.Request.Reset {
		for _, p := range job.Request.Packs {
			if err := publishFile(job.ID, p); err != nil {
				return fail(err)
			}
		}
	}
	err = transaction.WithTransaction(ctx, func(tx context.Context) (any, error) {
		if err := checkVersion(tx, job.Request.ExpectedVersion); err != nil {
			return nil, err
		}
		for i, p := range job.Request.Packs {
			if err := version.DeleteMasterDb(tx, p.Type); err != nil {
				return nil, err
			}
			batch := make([]map[string]any, 0, 256)
			flush := func() error { err := version.InsertMasterDb(tx, p.Type, batch); batch = batch[:0]; return err }
			err := crypto.IterateMasterDb(tx, filepath.Join(config.GetDBDir(), p.FileName), packs[i].CryptoKey, p.Type, func(row map[string]any) error {
				batch = append(batch, row)
				if len(batch) == cap(batch) {
					return flush()
				}
				return nil
			})
			if err != nil {
				return nil, err
			}
			if err := flush(); err != nil {
				return nil, err
			}
			if err := opsolis.UpsertMasterTagPack(tx, packs[i]); err != nil {
				return nil, err
			}
		}
		if err := opsolis.UpdateServerProfileMasterVersion(tx, job.Version); err != nil {
			return nil, err
		}
		if err := version.UpsertMasterVersion(tx, job.Version); err != nil {
			return nil, err
		}
		committed := *job
		committed.Committed = true
		committed.State = "committed"
		committed.UpdatedAt = time.Now().UnixMilli()
		return nil, opsolis.UpdateMasterImportJob(tx, &committed)
	})
	if err != nil {
		return fail(err)
	}
	job.Committed = true
	job.State = "committed"
	if err := RefreshSnapshot(ctx); err != nil {
		resolved = false
		return err
	}
	return c.cleanup(ctx, job)
}

func checkVersion(ctx context.Context, expected string) error {
	profile, err := opsolis.FindServerProfile(ctx)
	if err != nil {
		return err
	}
	if profile.MasterVersion != expected {
		return ErrStaleMaster
	}
	if profile.MasterVersionOrig == "" {
		return errors.New("original master baseline is missing; perform an official update first")
	}
	v, err := version.GetMasterVersion(ctx)
	if err != nil {
		return err
	}
	if v != expected {
		return errors.New("master version documents disagree")
	}
	return nil
}

func preparePublication(ctx context.Context, job *ImportJob) ([]*ptransaction.MasterTagPack, error) {
	if err := checkVersion(ctx, job.Request.ExpectedVersion); err != nil {
		return nil, err
	}
	active, err := opsolis.GetAllMasterTagPack(ctx)
	if err != nil {
		return nil, err
	}
	byType := make(map[string]*ptransaction.MasterTagPack, len(active))
	for _, p := range active {
		byType[p.Type] = p
	}
	if job.Request.Reset {
		return prepareResetPublication(ctx, job, byType)
	}
	if err := validateOriginalBaseline(ctx, byType); err != nil {
		return nil, err
	}
	root := config.GetDBDir()
	if err := makeSpool(job.ID); err != nil {
		return nil, err
	}
	result := make([]*ptransaction.MasterTagPack, 0, len(job.Request.Packs))
	for _, p := range job.Request.Packs {
		old := byType[p.Type]
		if old == nil || old.FileName != p.BaseFileName {
			return nil, ErrStaleMaster
		}
		source, err := safeFile(root, filepath.Join(".master-staging", job.ID, p.FileName))
		if err != nil {
			return nil, err
		}
		spool := spoolRelative(job.ID, p.FileName)
		err = copyChecked(root, spool, source, p)
		source.Close()
		if err != nil {
			return nil, err
		}
		oldPath := filepath.Join(root, old.FileName)
		newPath := filepath.Join(root, spool)
		if err := crypto.ValidateMasterDb(ctx, oldPath, newPath, old.CryptoKey, p.Type); err != nil {
			return nil, fmt.Errorf("%s encrypted schema: %w", p.Type, err)
		}
		count, err := ValidatePack(ctx, newPath, old.CryptoKey, p.Type)
		if err != nil {
			return nil, err
		}
		if count != p.RowCount {
			return nil, fmt.Errorf("%s row count changed after staging", p.Type)
		}
		if count == 0 && !job.Request.AllowEmpty {
			var nonempty bool
			if err := crypto.IterateMasterDb(ctx, oldPath, old.CryptoKey, p.Type, func(map[string]any) error { nonempty = true; return nil }); err != nil {
				return nil, err
			}
			if nonempty {
				return nil, fmt.Errorf("emptying %s requires --allow-empty", p.Type)
			}
		}
		next := proto.Clone(old).(*ptransaction.MasterTagPack)
		next.FileName = p.FileName
		next.FileSize = int32(p.FileSize)
		u, err := url.Parse(next.DownloadUrl)
		if err != nil || u.Host == "" {
			return nil, fmt.Errorf("invalid download URL for %s", p.Type)
		}
		u.Path = path.Join(path.Dir(u.Path), p.FileName)
		u.RawPath = ""
		next.DownloadUrl = u.String()
		result = append(result, next)
	}
	return result, nil
}

func spoolRelative(id, name string) string { return filepath.Join(".master-publication", id, name) }

// All untrusted path components have fixed syntax. Reject symlinks on every
// component as well as non-regular files, and constrain opens to an os.Root.
func safeFile(root, relative string) (*os.File, error) {
	r, err := os.OpenRoot(root)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	if !filepath.IsLocal(relative) {
		return nil, errors.New("non-local master path")
	}
	for current := relative; current != "."; current = filepath.Dir(current) {
		st, err := r.Lstat(current)
		if err != nil {
			return nil, err
		}
		if st.Mode()&os.ModeSymlink != 0 {
			return nil, errors.New("symlink master paths are forbidden")
		}
		if current == relative && !st.Mode().IsRegular() {
			return nil, errors.New("master file is not regular")
		}
	}
	return r.Open(relative)
}

func makeSpool(id string) error {
	r, err := os.OpenRoot(config.GetDBDir())
	if err != nil {
		return err
	}
	defer r.Close()
	for _, dir := range []string{".master-publication", filepath.Join(".master-publication", id)} {
		if err := r.Mkdir(dir, 0700); err != nil && !errors.Is(err, os.ErrExist) {
			return err
		}
		st, err := r.Lstat(dir)
		if err != nil {
			return err
		}
		if !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
			return errors.New("invalid master publication directory")
		}
	}
	return nil
}

func copyChecked(root, relative string, source io.Reader, p StagedPack) (retErr error) {
	r, err := os.OpenRoot(root)
	if err != nil {
		return err
	}
	defer r.Close()
	f, err := r.OpenFile(relative, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer func() { retErr = errors.Join(retErr, f.Close()) }()
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(f, h), io.LimitReader(source, p.FileSize+1))
	if err != nil {
		return err
	}
	if n != p.FileSize || hex.EncodeToString(h.Sum(nil)) != p.FileName {
		return errors.New("staged master hash or size mismatch")
	}
	return f.Sync()
}

func publishFile(id string, p StagedPack) error {
	root := config.GetDBDir()
	source, err := safeFile(root, spoolRelative(id, p.FileName))
	if err != nil {
		return err
	}
	defer source.Close()
	// A hard link publishes the fully synced private copy atomically. If recovery/retry
	// finds the same name, verify its bytes instead of replacing it.
	r, err := os.OpenRoot(root)
	if err != nil {
		return err
	}
	defer r.Close()
	err = r.Link(spoolRelative(id, p.FileName), p.FileName)
	if errors.Is(err, os.ErrExist) {
		f, e := safeFile(root, p.FileName)
		if e != nil {
			return e
		}
		defer f.Close()
		h := sha256.New()
		n, e := io.Copy(h, f)
		if e != nil {
			return e
		}
		if n != p.FileSize || hex.EncodeToString(h.Sum(nil)) != p.FileName {
			return errors.New("existing destination does not match staged pack")
		}
		return nil
	}
	if err != nil {
		return err
	}
	return syncDirectory(root)
}

func syncDirectory(dir string) error {
	f, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}

func deleteUnreferenced(ctx context.Context, name string, contentAddressed bool) error {
	if !hashName.MatchString(name) {
		return errors.New("invalid obsolete master filename")
	}
	active, err := opsolis.GetAllMasterTagPack(ctx)
	if err != nil {
		return err
	}
	originals, err := opsolis.GetAllOriginalMasterTagPack(ctx)
	if err != nil {
		return err
	}
	for _, p := range append(active, originals...) {
		if p.FileName == name {
			return nil
		}
	}
	f, err := safeFile(config.GetDBDir(), name)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if contentAddressed {
		h := sha256.New()
		_, hashErr := io.Copy(h, f)
		if hashErr != nil {
			f.Close()
			return hashErr
		}
		if hex.EncodeToString(h.Sum(nil)) != name {
			f.Close()
			return errors.New("refusing to delete an unowned file at the staged master destination")
		}
	}
	f.Close()
	r, err := os.OpenRoot(config.GetDBDir())
	if err != nil {
		return err
	}
	defer r.Close()
	if err := r.Remove(name); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return syncDirectory(config.GetDBDir())
}

func cleanupUncommitted(ctx context.Context, job *ImportJob) error {
	// Reset never creates custom files. Restored originals remain available
	// even when publication fails or its outcome is recovered after a crash.
	if job.Request.Reset {
		return nil
	}
	for _, p := range job.Request.Packs {
		if err := deleteUnreferenced(ctx, p.FileName, true); err != nil {
			return err
		}
	}
	return removeStaging(job)
}

func removeStaging(job *ImportJob) error {
	if job.Request.Reset {
		return nil
	}
	r, err := os.OpenRoot(config.GetDBDir())
	if err != nil {
		return err
	}
	defer r.Close()
	var failures []error
	for _, base := range []string{".master-staging", ".master-publication"} {
		for _, p := range job.Request.Packs {
			rel := filepath.Join(base, job.ID, p.FileName)
			f, err := safeFile(config.GetDBDir(), rel)
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			if err != nil {
				failures = append(failures, err)
				continue
			}
			f.Close()
			if err := r.Remove(rel); err != nil && !errors.Is(err, os.ErrNotExist) {
				failures = append(failures, err)
			}
		}
		// Never recursively remove unknown files in a staging directory.
		if err := r.Remove(filepath.Join(base, job.ID)); err != nil && !errors.Is(err, os.ErrNotExist) {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

func (c *Coordinator) cleanup(ctx context.Context, job *ImportJob) error {
	var cleanupErr error
	for _, p := range job.Request.Packs {
		cleanupErr = errors.Join(cleanupErr, deleteUnreferenced(ctx, p.BaseFileName, false))
	}
	cleanupErr = errors.Join(cleanupErr, removeStaging(job))
	job.State = "complete"
	job.Error = ""
	if cleanupErr != nil {
		job.State = "cleanup_pending"
		job.Error = cleanupErr.Error()
	}
	job.UpdatedAt = time.Now().UnixMilli()
	return errors.Join(cleanupErr, opsolis.UpdateMasterImportJob(ctx, job))
}

// Recover runs before startup's official updater and before serving traffic.
// Failed/queued jobs are not replayed: the old version is preserved. Committed
// jobs finish only targeted cleanup; they never undo a published transaction.
func (c *Coordinator) Recover(ctx context.Context) error {
	jobs, err := opsolis.PendingMasterImportJobs(ctx)
	if err != nil {
		return err
	}
	for _, job := range jobs {
		if job.ID != job.Request.ID {
			return errors.New("master publication job identity is inconsistent")
		}
		if err := validateRequest(job.Request); err != nil {
			return err
		}
		if !job.Committed {
			job, err = opsolis.FenceFailedMasterImport(ctx, job.ID, "publication interrupted before commit", time.Now().UnixMilli())
			if err != nil {
				return err
			}
		}
		if job.Committed {
			if err := c.cleanup(ctx, job); err != nil {
				zap.L().Warn("master published; cleanup pending", zap.String("job", job.ID), zap.Error(err))
			}
		} else {
			if err := cleanupUncommitted(ctx, job); err != nil {
				return err
			}
			job.State = "failed"
			job.UpdatedAt = time.Now().UnixMilli()
			if err := opsolis.UpdateMasterImportJob(ctx, job); err != nil {
				return err
			}
		}
	}
	return nil
}
