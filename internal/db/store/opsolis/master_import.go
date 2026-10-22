package opsolis

import (
	"context"
	"fmt"
	"github.com/vertesan/solis-server/internal/db/internal/depot"
	"github.com/vertesan/solis-server/internal/db/model/solis/smodel"
	"github.com/vertesan/solis-server/internal/db/model/solis/sname"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readconcern"
	"go.mongodb.org/mongo-driver/v2/mongo/writeconcern"
)

func InsertMasterImportJob(ctx context.Context, job *smodel.MasterImportJob) error {
	_, err := depot.Solis.Collection(sname.MasterImportJob, options.Collection().SetWriteConcern(writeconcern.Majority())).InsertOne(ctx, job)
	return err
}

func UpdateMasterImportJob(ctx context.Context, job *smodel.MasterImportJob) error {
	result, err := depot.Solis.Collection(sname.MasterImportJob, options.Collection().SetWriteConcern(writeconcern.Majority())).ReplaceOne(ctx, bson.M{"_id": job.ID}, job)
	if err != nil {
		return err
	}
	if result.MatchedCount != 1 {
		return fmt.Errorf("master publication job %s is missing", job.ID)
	}
	return nil
}

// Majority visibility is essential when resolving an uncertain transaction commit.
func FindMasterImportJob(ctx context.Context, id string) (*smodel.MasterImportJob, error) {
	var job smodel.MasterImportJob
	err := depot.Solis.Collection(sname.MasterImportJob, options.Collection().SetReadConcern(readconcern.Majority())).FindOne(ctx, bson.M{"_id": id}).Decode(&job)
	return &job, err
}

func PendingMasterImportJobs(ctx context.Context) ([]*smodel.MasterImportJob, error) {
	cursor, err := depot.Solis.Collection(sname.MasterImportJob, options.Collection().SetReadConcern(readconcern.Majority())).Find(ctx, bson.M{"state": bson.M{"$nin": []string{"complete", "failed"}}})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var jobs []*smodel.MasterImportJob
	err = cursor.All(ctx, &jobs)
	return jobs, err
}

// FenceFailedMasterImport conflicts with the transaction's write to this same
// job. A successful majority write proves a late/ambiguous commit cannot still
// publish it. If that transaction already committed, the predicate cannot match.
func FenceFailedMasterImport(ctx context.Context, id, message string, now int64) (*smodel.MasterImportJob, error) {
	_, err := depot.Solis.Collection(sname.MasterImportJob, options.Collection().SetWriteConcern(writeconcern.Majority())).UpdateOne(ctx,
		bson.M{"_id": id, "committed": bson.M{"$ne": true}},
		bson.M{"$set": bson.M{"state": "abort_cleanup_pending", "error": message, "updatedAt": now}})
	if err != nil {
		return nil, err
	}
	return FindMasterImportJob(ctx, id)
}
