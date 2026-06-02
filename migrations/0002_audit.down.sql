-- 0002_audit.down.sql
DROP TRIGGER IF EXISTS trg_auditoria_imutavel ON auditoria;
DROP TRIGGER IF EXISTS trg_audit_alertas      ON alertas;
DROP TRIGGER IF EXISTS trg_audit_contas       ON contas;
DROP TRIGGER IF EXISTS trg_audit_transacoes   ON transacoes;

DROP FUNCTION IF EXISTS fn_bloqueia_alteracao_auditoria();
DROP FUNCTION IF EXISTS fn_auditoria();

DROP TABLE IF EXISTS auditoria;