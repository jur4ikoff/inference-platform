module github.com/jur4ikoff/model-service

go 1.26.5

require (
	github.com/danielgtaylor/huma/v2 v2.39.1
	github.com/google/uuid v1.6.0
	github.com/jackc/pgx/v5 v5.11.0
	github.com/jur4ikoff/internal-api v0.0.0
	github.com/jur4ikoff/utils v0.0.0
	google.golang.org/grpc v1.81.1
)

replace github.com/jur4ikoff/internal-api => ../internal-api
replace github.com/jur4ikoff/utils => ../utils

require (
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	golang.org/x/net v0.57.0 // indirect
	golang.org/x/sync v0.22.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/text v0.40.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260414002931-afd174a4e478 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
)
