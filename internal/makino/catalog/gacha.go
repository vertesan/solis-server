package catalog

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"slices"

	"github.com/vertesan/solis-server/internal/db/model/solis/smodel"
	"github.com/vertesan/solis-server/internal/db/store/opsolis"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"go.yaml.in/yaml/v3"
)

// ApplyGacha changes the roster or rules without changing official master data.
func ApplyGacha(ctx context.Context, kind, mode string, documents [][]byte) error {
	if kind != "gacha" && kind != "gacha-rules" {
		return fmt.Errorf("unknown gacha catalog %q", kind)
	}
	if kind == "gacha-rules" {
		if mode != "overwrite" {
			return errors.New("gacha-rules requires overwrite")
		}
		var rules smodel.GachaRules
		if err := decodeSingleGachaDocument(documents, &rules); err != nil {
			return err
		}
		if err := rules.Validate(); err != nil {
			return err
		}
		return transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
			if err := ValidateGachaRulesReferences(ctx, &rules); err != nil {
				return nil, err
			}
			rotation, err := opsolis.FindGachaCatalog(ctx)
			if err != nil {
				return nil, err
			}
			if err := validateExplicitGachaRoster(ctx, rotation, &rules); err != nil {
				return nil, err
			}
			return nil, opsolis.UpsertGachaRules(ctx, &rules)
		})
	}
	input := &smodel.GachaCatalog{IgnoreOfficialDates: true}
	switch mode {
	case "clear":
		if len(documents) != 0 {
			return errors.New("clear does not accept input files")
		}
		input.GachaIds = []string{}
	case "overwrite", "add":
		if err := decodeSingleGachaDocument(documents, input); err != nil {
			return err
		}
		if len(input.GachaIds) == 0 && (mode == "add" || !input.UseMasterCatalog) {
			return errors.New("gacha roster is empty; use clear to disable all gachas")
		}
	default:
		return fmt.Errorf("unknown gacha operation %q", mode)
	}
	if err := input.Validate(); err != nil {
		return err
	}
	return transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		result := input
		if mode == "add" {
			current, err := opsolis.FindGachaCatalog(ctx)
			if err != nil {
				return nil, err
			}
			for _, id := range input.GachaIds {
				if !slices.Contains(current.GachaIds, id) {
					current.GachaIds = append(current.GachaIds, id)
				}
			}
			// Add keeps the roster's existing date policy and automatic mode.
			result = current
		}
		if len(result.GachaIds) > 0 {
			rules, err := opsolis.FindGachaRules(ctx)
			if err != nil {
				return nil, err
			}
			if err := validateExplicitGachaRoster(ctx, result, rules); err != nil {
				return nil, err
			}
		}
		return nil, opsolis.UpsertGachaCatalog(ctx, result)
	})
}

func ExportGacha(ctx context.Context, kind string) ([]byte, error) {
	switch kind {
	case "gacha":
		value, err := opsolis.FindGachaCatalog(ctx)
		if err != nil {
			return nil, err
		}
		return yaml.Marshal(value)
	case "gacha-rules":
		value, err := opsolis.FindGachaRules(ctx)
		if err != nil {
			return nil, err
		}
		return yaml.Marshal(value)
	default:
		return nil, fmt.Errorf("unknown gacha catalog %q", kind)
	}
}

func decodeSingleGachaDocument(documents [][]byte, target any) error {
	if len(documents) != 1 {
		return errors.New("gacha configuration requires exactly one YAML file")
	}
	decoder := yaml.NewDecoder(bytes.NewReader(documents[0]))
	decoder.KnownFields(true)
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("decode gacha configuration: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("gacha configuration requires exactly one YAML document")
	}
	return nil
}
