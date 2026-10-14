module github.com/medxops/trazr-otel/trazrextension

go 1.27.2

require (
	github.com/golang-jwt/jwt/v5 v5.3.1
	github.com/gorilla/websocket v1.5.3
	github.com/jackc/pgx/v5 v5.11.0
	github.com/medxops/trazr-otel/pkg/auth v0.0.0-00010101000000-000000000000
	github.com/medxops/trazr-otel/pkg/event v0.0.0-00010101000000-000000000000
	github.com/medxops/trazr-otel/pkg/metrics v0.0.0
	github.com/medxops/trazr-otel/pkg/repository v0.0.0-00010101000000-000000000000
	github.com/medxops/trazr-otel/pkg/util v0.0.0-00010101000000-000000000000
	github.com/stretchr/testify v1.12.1
	go.opentelemetry.io/collector/component v1.69.0
	go.opentelemetry.io/collector/extension v1.69.0
	go.opentelemetry.io/otel/trace v1.47.0
	go.uber.org/zap v1.28.0
)

replace github.com/medxops/trazr-otel/pkg/auth => ../pkg/auth

replace github.com/medxops/trazr-otel/pkg/repository => ../pkg/repository

replace github.com/medxops/trazr-otel/pkg/metrics => ../pkg/metrics

replace github.com/medxops/trazr-otel/pkg/util => ../pkg/util

replace github.com/medxops/trazr-otel/pkg/event => ../pkg/event

replace github.com/medxops/trazr-otel/pkg/config => ../pkg/config

require (
	cloud.google.com/go/compute/metadata v0.9.0 // indirect
	dario.cat/mergo v1.0.2 // indirect
	github.com/Masterminds/goutils v1.1.1 // indirect
	github.com/Masterminds/semver/v3 v3.3.0 // indirect
	github.com/Masterminds/sprig/v3 v3.3.0 // indirect
	github.com/apapsch/go-jsonmerge/v2 v2.0.0 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/boombuler/barcode v1.0.1-0.20190219062509-6c824513bacc // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/coreos/go-oidc v2.5.0+incompatible // indirect
	github.com/deepmap/oapi-codegen/v2 v2.1.0 // indirect
	github.com/dghubble/sessions v0.4.1 // indirect
	github.com/dlclark/regexp2 v1.10.0 // indirect
	github.com/go-logr/logr v1.4.4 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-ole/go-ole v1.3.0 // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/gorilla/securecookie v1.1.2 // indirect
	github.com/hashicorp/go-version v1.9.0 // indirect
	github.com/huandu/xstrings v1.5.0 // indirect
	github.com/iancoleman/strcase v0.3.0 // indirect
	github.com/jackc/chunkreader/v2 v2.0.1 // indirect
	github.com/jackc/pgconn v1.14.3 // indirect
	github.com/jackc/pgio v1.0.0 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgproto3/v2 v2.3.3 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	github.com/jeremywohl/flatten v1.0.1 // indirect
	github.com/joho/godotenv v1.5.1 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/klauspost/compress v1.20.1 // indirect
	github.com/lib/pq v1.12.3 // indirect
	github.com/lufia/plan9stats v0.0.0-20260330125221-c963978e514e // indirect
	github.com/mitchellh/copystructure v1.2.0 // indirect
	github.com/mitchellh/reflectwalk v1.0.2 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.3-0.20250322232337-35a7c28c31ee // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/nats-io/nats.go v1.54.0 // indirect
	github.com/nats-io/nkeys v0.4.16 // indirect
	github.com/nats-io/nuid v1.0.1 // indirect
	github.com/oapi-codegen/runtime v1.1.1 // indirect
	github.com/pinecone-io/go-pinecone v0.4.1 // indirect
	github.com/pkoukk/tiktoken-go v0.1.8 // indirect
	github.com/power-devops/perfstat v0.0.0-20240221224432-82ca36839d55 // indirect
	github.com/pquerna/cachecontrol v0.2.0 // indirect
	github.com/pquerna/otp v1.5.0 // indirect
	github.com/prometheus/client_golang v1.25.0 // indirect
	github.com/prometheus/client_model v0.6.3 // indirect
	github.com/prometheus/common v0.72.1 // indirect
	github.com/prometheus/procfs v0.22.0 // indirect
	github.com/qdrant/go-client v1.19.4 // indirect
	github.com/shirou/gopsutil/v3 v3.24.5 // indirect
	github.com/shoenig/go-m1cpu v0.2.4 // indirect
	github.com/shopspring/decimal v1.4.0 // indirect
	github.com/spf13/cast v1.7.0 // indirect
	github.com/tklauser/go-sysconf v0.4.0 // indirect
	github.com/tklauser/numcpus v0.12.0 // indirect
	github.com/tmc/langchaingo v0.1.15 // indirect
	github.com/valyala/bytebufferpool v1.0.0 // indirect
	github.com/valyala/fasttemplate v1.2.2 // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/collector/featuregate v1.69.0 // indirect
	go.opentelemetry.io/collector/internal/componentalias v0.163.0 // indirect
	go.opentelemetry.io/collector/pdata v1.69.0 // indirect
	go.opentelemetry.io/collector/pdata/pprofile v0.163.0 // indirect
	go.opentelemetry.io/otel v1.47.0 // indirect
	go.opentelemetry.io/otel/log v1.47.0 // indirect
	go.opentelemetry.io/otel/metric v1.47.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.yaml.in/yaml/v3 v3.0.5 // indirect
	golang.org/x/crypto v0.58.0 // indirect
	golang.org/x/net v0.61.0 // indirect
	golang.org/x/oauth2 v0.37.0 // indirect
	golang.org/x/sync v0.24.0 // indirect
	golang.org/x/sys v0.49.0 // indirect
	golang.org/x/text v0.43.0 // indirect
	golang.org/x/time v0.16.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20260706201446-f0a921348800 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260706201446-f0a921348800 // indirect
	google.golang.org/grpc v1.84.0 // indirect
	google.golang.org/protobuf v1.36.12 // indirect
	gopkg.in/go-jose/go-jose.v2 v2.6.3 // indirect
)
