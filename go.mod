module github.com/ipfs-shipyard/thunderdome

go 1.19

require (
	contrib.go.opencensus.io/exporter/prometheus v0.4.2
	github.com/aws/aws-sdk-go v1.44.115
	github.com/gorilla/websocket v1.5.0
	github.com/ipfs/go-path v0.3.0
	github.com/kortschak/utter v1.5.0
	github.com/pkg/profile v1.6.0
	github.com/prometheus/client_golang v1.13.0
	github.com/prometheus/common v0.37.0
	github.com/spenczar/tdigest v2.1.0+incompatible
	github.com/urfave/cli/v2 v2.19.2
	go.opencensus.io v0.23.0
	go.opentelemetry.io/otel v1.11.0
	go.opentelemetry.io/otel/exporters/jaeger v1.11.0
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.11.0
	go.opentelemetry.io/otel/exporters/zipkin v1.11.0
	go.opentelemetry.io/otel/sdk v1.11.0
	go.opentelemetry.io/otel/trace v1.11.0
	golang.org/x/exp v0.0.0-20230213192124-5e25df0256eb
	golang.org/x/net v0.0.0-20220225172249-27dd8689420f
	golang.org/x/sync v0.0.0-20220601150217-0de741cfad7f
)

replace github.com/pkg/profile => github.com/iand/profile v0.0.0-20220825113751-13692ce5785f

require (
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cenkalti/backoff/v4 v4.1.3 // indirect
	github.com/cespare/xxhash/v2 v2.1.2 // indirect
	github.com/cpuguy83/go-md2man/v2 v2.0.2 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/go-kit/log v0.2.1 // indirect
	github.com/go-logfmt/logfmt v0.5.1 // indirect
	github.com/go-logr/logr v1.2.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/golang/protobuf v1.5.2 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.7.0 // indirect
	github.com/ipfs/go-cid v0.1.0 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/jpillora/backoff v1.0.0 // indirect
	github.com/klauspost/cpuid/v2 v2.0.4 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.1 // indirect
	github.com/minio/blake2b-simd v0.0.0-20160723061019-3f5f724cb5b1 // indirect
	github.com/minio/sha256-simd v1.0.0 // indirect
	github.com/mr-tron/base58 v1.2.0 // indirect
	github.com/multiformats/go-base32 v0.0.3 // indirect
	github.com/multiformats/go-base36 v0.1.0 // indirect
	github.com/multiformats/go-multibase v0.0.3 // indirect
	github.com/multiformats/go-multihash v0.0.15 // indirect
	github.com/multiformats/go-varint v0.0.6 // indirect
	github.com/mwitkow/go-conntrack v0.0.0-20190716064945-2f068394615f // indirect
	github.com/openzipkin/zipkin-go v0.4.0 // indirect
	github.com/prometheus/client_model v0.2.0 // indirect
	github.com/prometheus/procfs v0.8.0 // indirect
	github.com/prometheus/statsd_exporter v0.22.7 // indirect
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/xrash/smetrics v0.0.0-20201216005158-039620a65673 // indirect
	go.opentelemetry.io/otel/exporters/otlp/internal/retry v1.11.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.11.0 // indirect
	go.opentelemetry.io/proto/otlp v0.19.0 // indirect
	golang.org/x/crypto v0.0.0-20210920023735-84f357641f63 // indirect
	golang.org/x/oauth2 v0.0.0-20220223155221-ee480838109b // indirect
	golang.org/x/sys v0.1.0 // indirect
	golang.org/x/text v0.3.7 // indirect
	google.golang.org/appengine v1.6.6 // indirect
	google.golang.org/genproto v0.0.0-20211118181313-81c1377c94b1 // indirect
	google.golang.org/grpc v1.46.2 // indirect
	google.golang.org/protobuf v1.28.1 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
)
