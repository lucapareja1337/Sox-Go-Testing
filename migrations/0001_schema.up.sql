-- 0001_schema.up.sql
-- Esquema base do experimento: bancos, titulares (PF/PJ), contas, transações e alertas.

-- Tipos enumerados (controle de domínio dos valores).
CREATE TYPE tipo_titular     AS ENUM ('PF','PJ');
CREATE TYPE tipo_transacao   AS ENUM ('PIX','TED','DOC','TRANSFERENCIA','SAQUE','DEPOSITO');
CREATE TYPE status_transacao AS ENUM ('PENDENTE','APROVADA','REJEITADA','EM_ANALISE');
CREATE TYPE severidade       AS ENUM ('BAIXA','MEDIA','ALTA','CRITICA');

-- Instituição financeira.
CREATE TABLE bancos (
    id        BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    codigo    VARCHAR(4)   NOT NULL UNIQUE,   -- código COMPE (ex.: 001, 237, 341)
    nome      VARCHAR(120) NOT NULL,
    criado_em TIMESTAMPTZ  NOT NULL DEFAULT now()
);

-- Titular Pessoa Física.
CREATE TABLE pessoas_fisicas (
    id        BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    cpf       CHAR(11)     NOT NULL UNIQUE,
    nome      VARCHAR(120) NOT NULL,
    criado_em TIMESTAMPTZ  NOT NULL DEFAULT now()
);

-- Titular Pessoa Jurídica.
CREATE TABLE pessoas_juridicas (
    id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    cnpj          CHAR(14)     NOT NULL UNIQUE,
    razao_social  VARCHAR(160) NOT NULL,
    nome_fantasia VARCHAR(160),
    criado_em     TIMESTAMPTZ  NOT NULL DEFAULT now()
);

-- Conta: pertence a um banco e a EXATAMENTE um titular (PF ou PJ).
CREATE TABLE contas (
    id                 BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    banco_id           BIGINT       NOT NULL REFERENCES bancos(id),
    agencia            VARCHAR(6)   NOT NULL,
    numero             VARCHAR(20)  NOT NULL,
    titular_tipo       tipo_titular NOT NULL,
    pessoa_fisica_id   BIGINT REFERENCES pessoas_fisicas(id),
    pessoa_juridica_id BIGINT REFERENCES pessoas_juridicas(id),
    saldo_centavos     BIGINT       NOT NULL DEFAULT 0,
    criado_em          TIMESTAMPTZ  NOT NULL DEFAULT now(),
    -- Integridade referencial polimórfica: um e apenas um titular.
    CONSTRAINT chk_titular_exclusivo CHECK (
        (titular_tipo = 'PF' AND pessoa_fisica_id   IS NOT NULL AND pessoa_juridica_id IS NULL) OR
        (titular_tipo = 'PJ' AND pessoa_juridica_id IS NOT NULL AND pessoa_fisica_id   IS NULL)
    ),
    CONSTRAINT uq_conta UNIQUE (banco_id, agencia, numero)
);

-- Transação entre duas contas.
-- iniciado_por / aprovado_por sustentam a segregação de funções (SOX).
CREATE TABLE transacoes (
    id               BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    conta_origem_id  BIGINT           NOT NULL REFERENCES contas(id),
    conta_destino_id BIGINT           NOT NULL REFERENCES contas(id),
    valor_centavos   BIGINT           NOT NULL CHECK (valor_centavos > 0),
    tipo             tipo_transacao   NOT NULL,
    status           status_transacao NOT NULL DEFAULT 'PENDENTE',
    iniciado_por     VARCHAR(60)      NOT NULL,
    aprovado_por     VARCHAR(60),
    descricao        VARCHAR(200),
    ocorrido_em      TIMESTAMPTZ      NOT NULL DEFAULT now(),
    criado_em        TIMESTAMPTZ      NOT NULL DEFAULT now(),
    CONSTRAINT chk_contas_distintas CHECK (conta_origem_id <> conta_destino_id)
);

CREATE INDEX idx_tx_origem_data ON transacoes (conta_origem_id, ocorrido_em);
CREATE INDEX idx_tx_status      ON transacoes (status);

-- Alerta gerado pelo motor de monitoramento.
CREATE TABLE alertas (
    id           BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    transacao_id BIGINT      NOT NULL REFERENCES transacoes(id),
    regra        VARCHAR(10) NOT NULL,
    nome_regra   VARCHAR(80) NOT NULL,
    severidade   severidade  NOT NULL,
    detalhe      TEXT        NOT NULL,
    criado_em    TIMESTAMPTZ NOT NULL DEFAULT now(),
    revisado     BOOLEAN     NOT NULL DEFAULT FALSE,
    revisado_por VARCHAR(60),
    revisado_em  TIMESTAMPTZ
);

CREATE INDEX idx_alerta_tx ON alertas (transacao_id);