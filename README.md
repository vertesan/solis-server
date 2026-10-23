# Overview

A server backend implementation for **IDOLY PRIDE**, built with Go, gRPC,
HTTP, and MongoDB.

Approximately 90% of the code was written by LLMs. I recorded all the prompts I used in [.agent/prompts/prompt.md](.agents/prompts/prompt.md), feel free to check it.

Many multiplayer features are intentionally single-player: you see only yourself.

The included `makino` CLI lets you manage users, edit shops and gachas, and
customize master data without rebuilding the server.

See [gRPC API coverage](coverage.md) for implemented APIs and remaining gaps.

> [!IMPORTANT]
> This project is not affiliated with or endorsed by the official IDOLY PRIDE
> team. Our original code was written independently, not copied or stolen from
> the official source repository. Game assets and other third-party materials
> remain the property of their respective rights holders.

## TL;DR

With Docker and Docker Compose installed:

1. Prepare the [resources](#resources) and [secret files](#secrets) below.
2. Rename `.env.example` to `.env` and edit it using the
   [configuration notes](#configurations).
3. Build, start, and create your first account:

```sh
docker compose up --build -d
docker compose logs -f solis          # Wait for initialization; Ctrl+C leaves it running
docker compose exec solis /app/makino user create
```

Keep the returned credentials private. Client routing and certificate trust
must be configured separately.

## Resources

**Resources are not provided, you must obtain compatible game resources yourself.**
Only use materials you are entitled to use. A resource directory looks like:

```text
resources/
├── manifest/
│   ├── mastertag.json           # Master database manifest
│   └── OctoManifest.json        # Asset manifest
├── db/                         # Encrypted master packs, named by the manifest
├── assets/
│   ├── assetbundle/            # AssetBundles in their revision/type/object layout
│   └── resources/              # Resources in their revision/type/object layout
└── Photo/                      # Uploaded photos; created by the server
```

Both manifests are required. Match the client and resource versions to the
server profile; keep asset paths consistent with the manifests.

If you are using Docker, remember to change the directory's `owner:group` to `10001:10001`.

## Building

### Container image

From the repository root:

```sh
docker build -t solis-server:local .
```

The image contains `/app/solis` and `/app/makino`, but no game resources or
secrets. Compose builds this image and initializes MongoDB's replica set for you.

### Native binaries

Install the Go version specified in [go.mod](go.mod), plus a C compiler and libc
development headers (for example, `build-essential` on Debian/Ubuntu).
SQLCipher requires CGO:

```sh
CGO_ENABLED=1 go build -trimpath -o solis ./cmd/solis
CGO_ENABLED=1 go build -trimpath -o makino ./cmd/makino
```

## Configurations

Rename [`.env.example`](.env.example) to `.env` in the repository root, then
replace its placeholders. Use different strong values for `SOLIS_DB_PASSWORD`,
`SOLIS_ADMIN_AUTH`, and `SOLIS_JWT_HMAC_KEY`; `openssl rand -hex 32` generates one.

- **Docker Compose:** `.env` supplies the variables explicitly referenced in
  [compose.yaml](compose.yaml); the file itself is not mounted. Exported shell
  variables take precedence, following [Compose's interpolation rules](https://docs.docker.com/compose/how-tos/environment-variables/variable-interpolation/).
  Database host, resource path, and key paths inside the container are set by Compose.
- **Native execution:** both binaries load `.env` from the working directory,
  overriding existing environment variables. Set `SOLIS_RESOURCE_DIR`, the four
  JWT/TLS key paths, and a reachable database host/port for your machine.

`SOLIS_DB_USERNAME` and `SOLIS_DB_PASSWORD` are used to build the connection URI
and initialize Compose's MongoDB. Changing them later does not change an
existing database user's credentials.

For Compose, set `SOLIS_DB_DATA_DIR=./data/` or another host directory.
Resources, administrator YAMLs, and secrets use `./resources`, `./config`, and
`./secrets` by default; see `compose.yaml` to override these host paths.

On Linux, set `SOLIS_UID` and `SOLIS_GID` in `.env` to the numbers from `id -u`
and `id -g`. The container user must be able to read secrets and write resources
and config (its default identity is `10001:10001`). Create the directories with
`mkdir -p resources config/master`.

### Secrets

For a new deployment, generate separate JWT and TLS keys, plus a MongoDB
replica-set key. Do not overwrite keys from an existing installation:

```sh
(
  umask 077
  mkdir -p secrets
  openssl genpkey -algorithm RSA -pkeyopt rsa_keygen_bits:3072 -out secrets/jwt-private.pem
  openssl pkey -in secrets/jwt-private.pem -pubout -out secrets/jwt-public.pem
  openssl req -x509 -newkey rsa:3072 -sha256 -days 365 -nodes \
    -keyout secrets/grpc-private.pem -out secrets/grpc-public.pem \
    -subj '/CN=localhost' -addext 'subjectAltName=DNS:localhost,DNS:solis,IP:127.0.0.1'
  openssl rand -base64 756 > secrets/mongo-keyfile
)
chmod 600 .env
# change owner:group if you are using docker
chown -R 10001:10001 secrets
```

This certificate is for local testing; devices need matching endpoint names
and a trusted issuer. Compose mounts keys as [secrets](https://docs.docker.com/compose/how-tos/use-secrets/)
under `/run/secrets`. Keep host files private and readable by the container user.
For native use, point the JWT/TLS variables to these files directly.

## Usage

> [!WARNING]
> Do not serve this project on the public Internet. Authentication and
> authorization are not hardened, and defenses against abuse and attacks are
> incomplete. Use a trusted private environment and keep MongoDB and admin APIs
> inaccessible from the Internet.

### Solis server

```sh
docker compose up -d              # Start
docker compose logs -f solis      # View logs
docker compose down              # Stop; keeps persistent data
```

For native use, prepare an authenticated MongoDB replica set named `rs0`, then
run `./solis` from the directory containing `.env`. First startup imports data
and can take a while.

Game gRPC uses TLS and a custom codec on **50051**; HTTP authentication, assets,
and admin APIs use **50052**. HTTPS access to the HTTP endpoints needs a TLS
reverse proxy. Compose binds application ports to loopback by default; change
`SOLIS_BIND_ADDRESS` only for a trusted LAN. MongoDB has no published host port.

Back up MongoDB together with the matching `resources/`, `config/`, `.env`, and
secrets. Stop services before copying database files. Keep these backups private.

### Makino CLI

Run the CLI inside the server container:

```sh
docker compose exec solis /app/makino --help
```

The examples below use the native binary; in Docker, replace `./makino` with
`docker compose exec solis /app/makino`:

```sh
./makino user create --id ABCD1234
./makino user god --id ABCD1234 --message enjoy
./makino user delete --id ABCD1234

./makino server notice --overwrite --dir config/catalogs/notice
./makino master export --type Card --file config/master/Card.yaml
# Edit the exported YAML, then validate and publish it:
./makino master import --file config/master/Card.yaml --dry-run
./makino master import --file config/master/Card.yaml
./makino master reset --type Card
./makino custom photo
```

See the [catalog guide](config/catalogs/README.md) for notices, shops, exchanges,
and gachas, or `./makino --help` for more commands. Bulk master exports require
`--all --dir PATH`; avoid printing large exports directly to your terminal.

User commands require the running server and `SOLIS_ADMIN_AUTH`. Catalog/master
commands require MongoDB; master operations must share Solis's resource directory.
Imports coordinate with a running server, briefly pausing gameplay, or run offline
when it is stopped. Keep your edited YAMLs: newer official master releases replace
customizations.
