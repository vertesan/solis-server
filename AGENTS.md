# Solis Server

This project is a private server, simulates the backend server implementation for "IDOLY PRIDE", a Japanese idol management smartphone game. Written in Go with a dual-protocol architecture: gRPC for real-time game logic communication and HTTP for authentication, asset delivery, and admin operations.

## Architecture

```
solis-server/
├── cmd/
│   └── solis/
│       └── main.go             CLI entry point: loads .env, starts pprof (dev), calls server.StartServers()
├── internal/                   Private Go packages, all Go source files are supposed to be inside
│   ├── db/                     MongoDB layer ("solis", "master", and "user" databases)
│   │   ├── model/
│   │   │   ├── solis/sname/    Constant strings for solis DB collection names
│   │   │   ├── solis/smodel/   BSON model definitions for solis documents
│   │   │   ├── master/mname/   Constant strings for master DB collection names
│   │   │   ├── master/mmodel/  BSON model definitions for master data documents
│   │   │   ├── user/uname/     Constant strings for user DB collection names
│   │   │   └── user/umodel/    BSON model definitions for user data documents
│   │   ├── store/
│   │   │   ├── opuser/         User-specific data CRUD
│   │   │   ├── opmaster/       Master data queries
│   │   │   └── opsolis/        Server-global data CRUD
│   │   ├── internal/depot/     Shared MongoDB client and database handles
│   │   └── transaction/        MongoDB transaction wrapper with snapshot reads / majority writes
│   ├── makino/                 Game-specific logic used by gRPC and admin handlers
│   ├── server/                 Handlers
│   │   ├── rpc/                gRPC servers (:50051) for game-specific logic APIs
│   │   └── http/               HTTP/Fiber server (:50052) for auth, assets, and admin APIs
│   ├── proto/                  Generated Go protobuf packages
│   │   ├── octo/               Octo manifest database messages
│   │   ├── papi/               gRPC API messages and services
│   │   ├── penum/              Enums used in other protobuf packages
│   │   ├── pmaster/            Master data messages
│   │   └── ptransaction/       Shared common transaction messages, usually nested in papi messages
│   └── utils/                  Configuration, context, JWT, encryption algorithms and common helpers
├── proto/                      Protobuf source definitions (`*.proto`)
├── test/                       Integration test scenarios for gRPC and HTTP endpoints
├── .env                        Environment variables automatically loaded when the server starts
├── agents/                     Documents created by agents
├── resources/                  Encrypted master databases and Unity assetbundles
├── scripts/                    TypeScript/JS utilities for data migration and manifest updates
└── traffic/                    Captured realworld game client-server gRPC traffic converted to JSON
```

## Key Technologies

- **Go 1.26.5**
- **gRPC** (`google.golang.org/grpc`) with custom codec compression and protobuf serialization
- **Fiber v3** (`github.com/gofiber/fiber/v3`) for HTTP
- **MongoDB** (`go.mongodb.org/mongo-driver/v2`) as the primary datastore
- **zap** (`go.uber.org/zap`) for logging

## gRPC Services

All gRPC services are supposed to be implemented under `internal/server/rpc/servers/` directory. These services simulate game gRPC services, deceiving clients to successfully complete the communication.
Services generated from source proto files are defined in `internal/proto/papi`.

## HTTP Endpoints

HTTP APIs are defined under `internal/server/http/` directory:

- **game**: Simulated game HTTP APIs, deceive clients to successfully complete the communication
- **admin**: APIs which's paths start with `/admin/*`, provides management APIs for private server administrators

## MongoDB structure

Three databases: `solis`, `master`, `user`.

- **solis**: Server-wide global data
- **master**: Static game master data. Each collection (except `_version`) is supposed to have a corresponding model defined in `internal/db/model/master/mmodel`. The `data` field in each document contains raw protobuf binary data, and the corresponding unmarshalled protobuf data struct is defined in `internal/proto/pmaster`
- **user**: User-specific data. Each collection is supposed to have a corresponding model defined in `internal/db/model/user/umodel`. The `Data` generic field in `UserStruct` is supposed to be a protobuf struct used by the original game, usually defined in `internal/proto/ptransaction` and `internal/proto/papi`

You can use MongoDB MCP server to retrieve or manipulate data if you need.

## Logging Rules

Avoid using global logger (`zap.L()`) when possible.
In HTTP and gRPC handlers, a `*zap.Logger` is threaded in the `context` through the entire invocation, use `GetLogger(ctx context.Context) *zap.Logger` function in `internal/utils/mcontext` to retrieve logger instance in these cases.
In scopes where don't have context, such as utility functions, using global logger is acceptable.
Sugaring a logger is perfectly acceptable, our program is not that performance critical.

## Testing Rules

- Integration tests live in `test/`, each test function is supposed to test an entire scenario flow
- Write tests to make sure everything works as expect once you completed a scenario. If the test fails, fix the issue and test again until success
- Remember to do cleanups at the end of each test if mock data is created in the database during the test

## Tools

- You can use `jq` if you need to retrieve structured JSON data, or other tools if you think could come in handy

## Precautions

- NEVER try to read the full contents of files in the `proto/` directory, each file in that directory is supposed to be super huge which will DEFINITELY exceed your context window
- NEVER read the `data` field of any documents in the `master` database, this field contains raw protobuf binary data and may be very large
