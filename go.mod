module github.com/nlpfollower/deltamind/backend

go 1.23.1

replace github.com/nlpfollower/deltamind/database => ./../database

replace github.com/nlpfollower/deltamind/nexus => ./../nexus

replace github.com/nlpfollower/deltamind/orchestration => ./../orchestration

require (
	github.com/golang-jwt/jwt/v5 v5.2.1
	github.com/gorilla/mux v1.8.1
	github.com/nlpfollower/deltamind/database v0.0.0
	github.com/nlpfollower/deltamind/nexus v0.0.0-00010101000000-000000000000
	github.com/nlpfollower/deltamind/orchestration v0.0.0-00010101000000-000000000000
	github.com/pkg/errors v0.9.1
	github.com/spf13/cobra v1.8.1
	github.com/stretchr/testify v1.9.0
	golang.org/x/crypto v0.28.0
	golang.org/x/net v0.25.0
)

require (
	github.com/boltdb/bolt v1.3.1 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/sashabaranov/go-openai v1.32.3 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	golang.org/x/sys v0.26.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
