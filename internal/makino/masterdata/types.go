// Package masterdata stages and publishes administrator-customized master packs.
package masterdata

import "github.com/vertesan/solis-server/internal/db/model/solis/smodel"

type ImportRequest = smodel.MasterImportRequest
type StagedPack = smodel.StagedMasterPack
type ImportJob = smodel.MasterImportJob
