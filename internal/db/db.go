// Package db concentra o acesso ao PostgreSQL via database/sql + lib/pq.
package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/go_lang_testing/banco-sox/internal/monitor"
	_ "github.com/lib/pq"
)

// DB envolve a conexão e expõe operações do experimento.
type DB struct {
	conn *sql.DB
}

// Abrir conecta ao banco e valida a conexão.
func Abrir(dsn string) (*DB, error) {
	conn, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("abrir conexão: %w", err)
	}
	conn.SetMaxOpenConns(10)
	conn.SetConnMaxLifetime(5 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := conn.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("ping: %w", err)
	}
	return &DB{conn: conn}, nil
}

// Fechar encerra a conexão.
func (d *DB) Fechar() error { return d.conn.Close() }

// ContaInfo traz o id e o tipo de titular de uma conta.
type ContaInfo struct {
	ID          int64
	TitularTipo monitor.TitularTipo
}

// ListarContas retorna todas as contas com seus tipos de titular.
func (d *DB) ListarContas(ctx context.Context) ([]ContaInfo, error) {
	rows, err := d.conn.QueryContext(ctx,
		`SELECT id, titular_tipo FROM contas ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ContaInfo
	for rows.Next() {
		var c ContaInfo
		var tt string
		if err := rows.Scan(&c.ID, &tt); err != nil {
			return nil, err
		}
		c.TitularTipo = monitor.TitularTipo(tt)
		out = append(out, c)
	}
	return out, rows.Err()
}

// BaselineConta calcula média e desvio-padrão históricos do valor das
// transações já APROVADAS de uma conta de origem.
func (d *DB) BaselineConta(ctx context.Context, contaOrigemID int64) (monitor.Baseline, error) {
	var media, desvio sql.NullFloat64
	var n int
	err := d.conn.QueryRowContext(ctx, `
		SELECT COALESCE(AVG(valor_centavos),0),
		       COALESCE(STDDEV_POP(valor_centavos),0),
		       COUNT(*)
		  FROM transacoes
		 WHERE conta_origem_id = $1
		   AND status = 'APROVADA'`, contaOrigemID).Scan(&media, &desvio, &n)
	if err != nil {
		return monitor.Baseline{}, err
	}
	return monitor.Baseline{
		MediaCentavos:  media.Float64,
		DesvioCentavos: desvio.Float64,
		Amostras:       n,
	}, nil
}

// TransacoesRecentes retorna as transações de uma conta dentro da janela.
func (d *DB) TransacoesRecentes(ctx context.Context, contaOrigemID int64, ref time.Time, janela time.Duration) ([]monitor.Transacao, error) {
	rows, err := d.conn.QueryContext(ctx, `
		SELECT id, conta_origem_id, valor_centavos, tipo, ocorrido_em,
		       iniciado_por, COALESCE(aprovado_por,'')
		  FROM transacoes
		 WHERE conta_origem_id = $1
		   AND ocorrido_em >= $2
		   AND ocorrido_em <  $3`,
		contaOrigemID, ref.Add(-janela), ref)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []monitor.Transacao
	for rows.Next() {
		var t monitor.Transacao
		if err := rows.Scan(&t.ID, &t.ContaOrigemID, &t.ValorCentavos, &t.Tipo,
			&t.OcorridoEm, &t.IniciadoPor, &t.AprovadoPor); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// InserirTransacao grava uma transação e devolve o id gerado.
func (d *DB) InserirTransacao(ctx context.Context, t monitor.Transacao, contaDestinoID int64, status, descricao string) (int64, error) {
	var aprov interface{}
	if t.AprovadoPor != "" {
		aprov = t.AprovadoPor
	}
	var id int64
	err := d.conn.QueryRowContext(ctx, `
		INSERT INTO transacoes
			(conta_origem_id, conta_destino_id, valor_centavos, tipo, status,
			 iniciado_por, aprovado_por, descricao, ocorrido_em)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		RETURNING id`,
		t.ContaOrigemID, contaDestinoID, t.ValorCentavos, t.Tipo, status,
		t.IniciadoPor, aprov, descricao, t.OcorridoEm).Scan(&id)
	return id, err
}

// InserirAlerta grava um alerta vinculado a uma transação.
func (d *DB) InserirAlerta(ctx context.Context, transacaoID int64, a monitor.Alerta) error {
	_, err := d.conn.ExecContext(ctx, `
		INSERT INTO alertas (transacao_id, regra, nome_regra, severidade, detalhe)
		VALUES ($1,$2,$3,$4,$5)`,
		transacaoID, a.Regra, a.NomeRegra, string(a.Severidade), a.Detalhe)
	return err
}

// MarcarEmAnalise atualiza o status da transação quando há alertas.
func (d *DB) MarcarEmAnalise(ctx context.Context, transacaoID int64) error {
	_, err := d.conn.ExecContext(ctx,
		`UPDATE transacoes SET status = 'EM_ANALISE' WHERE id = $1`, transacaoID)
	return err
}

// Conn expõe a conexão para pacotes auxiliares (ex.: relatório de auditoria).
func (d *DB) Conn() *sql.DB { return d.conn }
