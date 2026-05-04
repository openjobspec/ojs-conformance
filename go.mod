module github.com/openjobspec/ojs-conformance

go 1.25.0

require (
	github.com/openjobspec/ojs-proto v0.0.0
	github.com/redis/go-redis/v9 v9.17.3
	google.golang.org/grpc v1.81.0
	google.golang.org/protobuf v1.36.11
)

replace github.com/openjobspec/ojs-proto => ../ojs-proto

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	go.opentelemetry.io/otel/metric v1.43.0 // indirect
	go.opentelemetry.io/otel/sdk v1.43.0 // indirect
	go.opentelemetry.io/otel/trace v1.43.0 // indirect
	golang.org/x/net v0.51.0 // indirect
	golang.org/x/sys v0.42.0 // indirect
	golang.org/x/text v0.34.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260226221140-a57be14db171 // indirect
)
