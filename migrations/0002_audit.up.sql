-- 0002_audit.up.sql
-- Trilha de auditoria (SOX Seção 802): registro imutável de toda mudança nas
-- tabelas sensíveis. Implementada no banco (não na aplicação) para que NENHUMA
-- operação escape ao registro, mesmo alterações feitas por fora do código Go.

CREATE TABLE auditoria (
    id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tabela        VARCHAR(40)  NOT NULL,
    operacao      VARCHAR(10)  NOT NULL,        -- INSERT / UPDATE / DELETE
    registro_id   TEXT,
    dados_antigos JSONB,
    dados_novos   JSONB,
    usuario_db    VARCHAR(60)  NOT NULL DEFAULT current_user,
    ocorrido_em   TIMESTAMPTZ  NOT NULL DEFAULT clock_timestamp()
);

CREATE INDEX idx_auditoria_tabela ON auditoria (tabela, ocorrido_em);

-- Função genérica de auditoria: captura o antes/depois em JSONB.
CREATE OR REPLACE FUNCTION fn_auditoria() RETURNS TRIGGER AS $$
BEGIN
    IF TG_OP = 'INSERT' THEN
        INSERT INTO auditoria (tabela, operacao, registro_id, dados_novos)
        VALUES (TG_TABLE_NAME, TG_OP, NEW.id::text, to_jsonb(NEW));
        RETURN NEW;
    ELSIF TG_OP = 'UPDATE' THEN
        INSERT INTO auditoria (tabela, operacao, registro_id, dados_antigos, dados_novos)
        VALUES (TG_TABLE_NAME, TG_OP, NEW.id::text, to_jsonb(OLD), to_jsonb(NEW));
        RETURN NEW;
    ELSIF TG_OP = 'DELETE' THEN
        INSERT INTO auditoria (tabela, operacao, registro_id, dados_antigos)
        VALUES (TG_TABLE_NAME, TG_OP, OLD.id::text, to_jsonb(OLD));
        RETURN OLD;
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

-- Aplica a auditoria às tabelas que afetam relatórios financeiros.
CREATE TRIGGER trg_audit_transacoes
    AFTER INSERT OR UPDATE OR DELETE ON transacoes
    FOR EACH ROW EXECUTE FUNCTION fn_auditoria();

CREATE TRIGGER trg_audit_contas
    AFTER INSERT OR UPDATE OR DELETE ON contas
    FOR EACH ROW EXECUTE FUNCTION fn_auditoria();

CREATE TRIGGER trg_audit_alertas
    AFTER INSERT OR UPDATE OR DELETE ON alertas
    FOR EACH ROW EXECUTE FUNCTION fn_auditoria();

-- Imutabilidade: bloqueia qualquer UPDATE/DELETE na própria trilha.
CREATE OR REPLACE FUNCTION fn_bloqueia_alteracao_auditoria() RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'Trilha de auditoria imutavel (SOX 802): operacao % proibida.', TG_OP;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_auditoria_imutavel
    BEFORE UPDATE OR DELETE ON auditoria
    FOR EACH ROW EXECUTE FUNCTION fn_bloqueia_alteracao_auditoria();