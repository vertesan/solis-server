set -eu

protoc -I=./proto --go_out=. --go_opt=module=github.com/vertesan/solis-server proto/octodb.proto
protoc -I=./proto --go_out=. --go_opt=module=github.com/vertesan/solis-server \
  proto/penum.proto proto/pmaster.proto proto/ptransaction.proto proto/papi.proto
protoc -I=./proto --go-grpc_out=. --go-grpc_opt=module=github.com/vertesan/solis-server proto/papi.proto

find ./internal/proto -type f -name "*.pb.go" -print0 | xargs -0I {} sed -i 's/,omitempty//g' {}
