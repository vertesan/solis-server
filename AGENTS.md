# Solis Server

This project is a private server, simulates the backend server implementation for "IDOLY PRIDE", a Japanese idol management smartphone game. Written in Go with a dual-protocol architecture: gRPC for real-time game communication and HTTP for authentication, asset delivery, and admin operations.

## Architecture

```
solis-server/
├── main.go                 Entry point: loads .env, starts pprof (dev), calls network.StartServer()
├── .env                    Environment variables automatically loaded when server starts
├── agents/                 Documents created by agents
├── crypto/                 Encryption algorithms for master DB, Unity aseetbundle manifest
├── db/                     MongoDB layer (3 databases: "solis" for server global data, "master" for static game master data, "user" for user-specific data)
│   ├── def/
│   │   ├── solis/sname/    Constant strings for solis DB collection names
│   │   ├── solis/sstruct/  BSON struct definitions for solis documents
│   │   ├── master/mname/   Constant strings for master DB collection names
│   │   ├── master/mstruct/ BSON struct definitions for master data documents, each wraps a DataHolder (raw protobuf bytes)
│   │   ├── user/uname/     Constant strings for user DB collection names
│   │   └── user/ustruct/   BSON struct definitions for user data documents
│   ├── store/
│   │   ├── opuser/         User-specific data CRUD
│   │   ├── opmaster/       Master data queries
│   │   └── opsolis/        Server global data CRUD
│   ├── internal/
│   │   └── depot/          Shared MongoDB client and database handles (Client, Solis, Master, User)
│   └── transaction/        MongoDB transaction wrapper with snapshot reads / majority writes
├── makino/                 Service layer, game-specific logic, usually invoked by gRPC services and admin API handlers. Not all handlers have a corresponding service (especially for simple APIs)
├── network/                The handler layer
│   ├── rpc/                gRPC servers (:50051) for game communication
│   └── http/               HTTP/Fiber server (:50052) for auth endpoints, asset downloads, admin API
├── proto/                  Protobuf definitions: enums, master data, transactions, API services
│   ├── papi/               Generated Go code for all gRPC API request/response messages and services
│   ├── penum/              Generated Go code for game enums (CardType, CharacterType, ParameterType, etc.)
│   ├── pmaster/            Generated Go code for master data structs (Card, Character, Skill, Ability, etc.)
│   ├── ptransaction/       Generated Go code for frequently nested messages used in requests and responses, consider it an extension to `papi`
│   └── *.proto             Source .proto files
├── utils/                  Config (env vars), structured logging
├── test/                   Integration test scenarios for gRPC and HTTP endpoints
├── traffic/                Captured real world game client - server gRPC communication traffic, converted to JSON format
├── script/                 TypeScript/JS utilities for data migration and manifest updates
└── resources/              Encrypted master databases and game assets
```

## Key Technologies

- **Go 1.26.2** with modules
- **gRPC** (`google.golang.org/grpc`) with custom codec compression and protobuf serialization
- **Fiber v3** (`github.com/gofiber/fiber/v3`) for HTTP
- **MongoDB** (`go.mongodb.org/mongo-driver/v2`) as the primary datastore
- **zap** (`go.uber.org/zap`) for logging, with helper functions wrapped in `vertesan/solis-server/utils/rich`

## gRPC Services

All gRPC services are supposed to be implemented under `network/rpc/servers/` directory. These services simulate game gRPC services, deceiving clients to successfully complete the communication.
Services generated from source proto file are defined in `proto/papi`.

## HTTP Endpoints

HTTP APIs are defined under `network/http/` directory:

- **game**: Simulated game HTTP APIs, deceive clients to successfully complete the communication
- **admin**: APIs which's paths start with `/admin/*`, provides management APIs for private server administrators

## MongoDB structure

Three databases: `solis`, `master`, `user`.

- **solis**: Server global data
- **master**: Static game master data. Each collection (except `_version`) is supposed to have a corresponding struct defined in `db/internal/mstruct`. The `data` field in each document contains raw protobuf binary data, the corresponding unmarshalled protobuf data struct is defined in `proto/pmaster`
- **user**: User-specific data. Each collection is supposed to have a corresponding struct defined in `db/internal/ustruct`. The `Data` generic field in `UserStruct` is supposed to be a protobuf struct used by original game, usually defined in `proto/ptransaction` and `proto/papi`

You can use MongoDB MCP server to retrieve or manipulate data if you need.

## Testing Rules

- Integration tests live in `test/`, each test function is supposed to test an entire scenario flow
- Write tests to make sure everything works as expect once you completed a scenario. If the test fails, fix the issue and test again until success
- Remember to do cleanups at the end of each test if mock data is created in the database during the test

## Tools

- You can use `jq` if you need to retrieve structured JSON data, or other tools if you think could come in handy

## Precautions

- NEVER try to read the full contents of files in the `proto/` directory, each file in that directory is supposed to be super huge which will DEFINITELY exceed your context window
- NEVER read the `data` field of any documents in the `master` database, this field contains raw protobuf binary data and may be very large
