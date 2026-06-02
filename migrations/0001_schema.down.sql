-- 0001_schema.down.sql
DROP TABLE IF EXISTS alertas;
DROP TABLE IF EXISTS transacoes;
DROP TABLE IF EXISTS contas;
DROP TABLE IF EXISTS pessoas_juridicas;
DROP TABLE IF EXISTS pessoas_fisicas;
DROP TABLE IF EXISTS bancos;

DROP TYPE IF EXISTS severidade;
DROP TYPE IF EXISTS status_transacao;
DROP TYPE IF EXISTS tipo_transacao;
DROP TYPE IF EXISTS tipo_titular;