# Makefile - orquestra o experimento de monitoramento de transações (SOX)
# Variáveis de conexão podem ser sobrescritas via ambiente ou arquivo .env.

export PGHOST     ?= localhost
export PGPORT     ?= 5432
export PGUSER     ?= banco
export PGPASSWORD ?= banco
export PGDATABASE ?= banco_sox
export PGSSLMODE  ?= disable

.PHONY: ajuda up down migrate-up migrate-down version bootstrap simular tudo test reset psql wait-db docker-tudo

ajuda:
	@echo "Alvos disponiveis:"
	@echo "  make up           - sobe SO o PostgreSQL (docker compose)"
	@echo "  make down         - derruba os containers"
	@echo "  make migrate-up   - aplica as migracoes (go run, no host)"
	@echo "  make migrate-down - reverte a ultima migracao"
	@echo "  make version      - mostra migracoes aplicadas"
	@echo "  make bootstrap    - cria entidades e historico de base"
	@echo "  make simular      - roda a simulacao + monitoramento"
	@echo "  make test         - testes unitarios do motor de regras"
	@echo "  make tudo         - up + espera + migrate-up + bootstrap + simular (host)"
	@echo "  make docker-tudo  - roda o ciclo inteiro DENTRO do Docker (build)"
	@echo "  make reset        - apaga o volume do banco (recomeca do zero)"
	@echo "  make psql         - abre o psql dentro do container"

# Sobe apenas o banco. (migrate/simulador do compose ficam para o make docker-tudo.)
up:
	docker compose up -d postgres

down:
	docker compose down

reset:
	docker compose down -v

# Espera o Postgres ficar saudavel antes de prosseguir.
wait-db:
	@echo "aguardando o PostgreSQL..."
	@until docker compose exec -T postgres pg_isready -U $(PGUSER) -d $(PGDATABASE) >/dev/null 2>&1; do sleep 1; done
	@echo "PostgreSQL pronto."

migrate-up:
	go run ./cmd/migrate up

migrate-down:
	go run ./cmd/migrate down

version:
	go run ./cmd/migrate version

bootstrap:
	go run ./cmd/simulador -bootstrap -run=false

simular:
	go run ./cmd/simulador

test:
	go test ./...

# Ciclo no host: banco em container, migracao/simulacao via go run local.
tudo: up wait-db migrate-up bootstrap simular

# Ciclo 100% em container: constroi a imagem e encadeia pg -> migrate -> simulador.
docker-tudo:
	docker compose up --build --abort-on-container-exit

psql:
	docker compose exec postgres psql -U $(PGUSER) -d $(PGDATABASE)