// Comando simulador prepara dados de base e simula um fluxo de transações
// bancárias, submetendo cada uma ao motor de monitoramento SOX.
//
// Uso:
//
//	go run ./cmd/simulador -bootstrap   # cria bancos, PF/PJ, contas e histórico
//	go run ./cmd/simulador              # simula transações e gera alertas
//	go run ./cmd/simulador -bootstrap -run
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"time"

	"github.com/go_lang_testing/banco-sox/internal/audit"
	"github.com/go_lang_testing/banco-sox/internal/config"
	"github.com/go_lang_testing/banco-sox/internal/db"
	"github.com/go_lang_testing/banco-sox/internal/monitor"
)

func main() {
	fBootstrap := flag.Bool("bootstrap", false, "insere dados de base (idempotente)")
	fRun := flag.Bool("run", true, "executa a simulação de transações")
	flag.Parse()

	cfg := config.Carregar()
	d, err := db.Abrir(cfg.DSN)
	checar(err)
	defer d.Fechar()

	ctx := context.Background()

	if *fBootstrap {
		fmt.Println("== bootstrap: criando entidades e histórico de base ==")
		checar(bootstrap(ctx, d))
	}
	if *fRun {
		fmt.Println("\n== simulação: avaliando transações pelo motor SOX ==")
		checar(simular(ctx, d, cfg))
	}
}

// ---------------------------------------------------------------------------
// Bootstrap: cria bancos, pessoas, contas e um histórico "normal" por conta.
// ---------------------------------------------------------------------------

func bootstrap(ctx context.Context, d *db.DB) error {
	conn := d.Conn()

	bancoID, err := upsertBanco(ctx, conn, "341", "Banco Exemplo S.A.")
	if err != nil {
		return err
	}

	// Pessoas Físicas
	pfs := []struct{ cpf, nome string }{
		{"11111111111", "Ana Souza"},
		{"22222222222", "Bruno Lima"},
		{"33333333333", "Carla Dias"},
	}
	var contasPF []int64
	for i, p := range pfs {
		pfID, err := upsertPF(ctx, conn, p.cpf, p.nome)
		if err != nil {
			return err
		}
		c, err := upsertContaPF(ctx, conn, bancoID, "0001", fmt.Sprintf("PF%05d", i+1), pfID)
		if err != nil {
			return err
		}
		contasPF = append(contasPF, c)
	}

	// Pessoas Jurídicas
	pjs := []struct{ cnpj, rs, nf string }{
		{"11111111000111", "Comércio Alfa LTDA", "Alfa"},
		{"22222222000122", "Indústria Beta S.A.", "Beta"},
	}
	var contasPJ []int64
	for i, p := range pjs {
		pjID, err := upsertPJ(ctx, conn, p.cnpj, p.rs, p.nf)
		if err != nil {
			return err
		}
		c, err := upsertContaPJ(ctx, conn, bancoID, "0001", fmt.Sprintf("PJ%05d", i+1), pjID)
		if err != nil {
			return err
		}
		contasPJ = append(contasPJ, c)
	}

	// Histórico normal: dá um "padrão" a cada conta para a regra de desvio (R2).
	rng := rand.New(rand.NewSource(42))
	agora := time.Now()
	for _, c := range contasPF {
		destino := outraConta(c, append(contasPF, contasPJ...))
		for k := 0; k < 20; k++ {
			val := int64(20000 + rng.Intn(180000)) // R$ 200 a R$ 2.000
			quando := diaUtilHorarioComercial(agora.AddDate(0, 0, -(k + 1)))
			if err := inserirHistorico(ctx, d, c, destino, val, monitor.PF, quando); err != nil {
				return err
			}
		}
	}
	for _, c := range contasPJ {
		destino := outraConta(c, append(contasPF, contasPJ...))
		for k := 0; k < 20; k++ {
			val := int64(500000 + rng.Intn(4500000)) // R$ 5.000 a R$ 50.000
			quando := diaUtilHorarioComercial(agora.AddDate(0, 0, -(k + 1)))
			if err := inserirHistorico(ctx, d, c, destino, val, monitor.PJ, quando); err != nil {
				return err
			}
		}
	}
	fmt.Printf("contas PF: %v | contas PJ: %v\n", contasPF, contasPJ)
	return nil
}

func inserirHistorico(ctx context.Context, d *db.DB, origem, destino, valor int64, tt monitor.TitularTipo, quando time.Time) error {
	tx := monitor.Transacao{
		ContaOrigemID: origem,
		ValorCentavos: valor,
		Tipo:          "PIX",
		OcorridoEm:    quando,
		IniciadoPor:   "operador.lote",
		AprovadoPor:   "gerente.lote",
		TitularTipo:   tt,
	}
	_, err := d.InserirTransacao(ctx, tx, destino, "APROVADA", "historico de base")
	return err
}

// ---------------------------------------------------------------------------
// Simulação: cenário com transações normais e anomalias plantadas.
// ---------------------------------------------------------------------------

func simular(ctx context.Context, d *db.DB, cfg config.Config) error {
	contas, err := d.ListarContas(ctx)
	if err != nil {
		return err
	}
	if len(contas) < 2 {
		return fmt.Errorf("sem contas suficientes; rode com -bootstrap primeiro")
	}

	pf := primeiraDoTipo(contas, monitor.PF)
	pj := primeiraDoTipo(contas, monitor.PJ)
	destino := outraContaInfo(pf.ID, contas)
	// Instante de referência determinístico (dia útil, horário comercial) para
	// que o cenário "normal" não dispare a regra de horário (R3).
	agora := referenciaUtil(time.Now())

	tipoDe := func(id int64) monitor.TitularTipo {
		for _, c := range contas {
			if c.ID == id {
				return c.TitularTipo
			}
		}
		return monitor.PF
	}

	// Cenários nomeados para deixar claro o que cada um deve disparar.
	cenarios := []struct {
		nome   string
		tx     monitor.Transacao
		espera string
	}{
		{
			nome: "normal PF",
			tx: monitor.Transacao{ContaOrigemID: pf.ID, ValorCentavos: 80000, Tipo: "PIX",
				OcorridoEm: agora, IniciadoPor: "op.ana", AprovadoPor: "ger.bruno", TitularTipo: tipoDe(pf.ID)},
			espera: "nenhum alerta",
		},
		{
			nome: "valor altissimo (R1)",
			tx: monitor.Transacao{ContaOrigemID: pf.ID, ValorCentavos: 8000000, Tipo: "TED",
				OcorridoEm: agora, IniciadoPor: "op.ana", AprovadoPor: "ger.bruno", TitularTipo: tipoDe(pf.ID)},
			espera: "R1 (+R2)",
		},
		{
			nome: "desvio do padrao PF (R2)",
			tx: monitor.Transacao{ContaOrigemID: pf.ID, ValorCentavos: 1500000, Tipo: "PIX",
				OcorridoEm: agora, IniciadoPor: "op.ana", AprovadoPor: "ger.bruno", TitularTipo: tipoDe(pf.ID)},
			espera: "R2",
		},
		{
			nome: "madrugada PJ (R3)",
			tx: monitor.Transacao{ContaOrigemID: pj.ID, ValorCentavos: 700000, Tipo: "TED",
				OcorridoEm:  time.Date(agora.Year(), agora.Month(), agora.Day(), 3, 12, 0, 0, agora.Location()),
				IniciadoPor: "op.carla", AprovadoPor: "ger.bruno", TitularTipo: tipoDe(pj.ID)},
			espera: "R3",
		},
		{
			nome: "alto valor sem segregacao (R5)",
			tx: monitor.Transacao{ContaOrigemID: pj.ID, ValorCentavos: 2000000, Tipo: "TED",
				OcorridoEm: agora, IniciadoPor: "op.carla", AprovadoPor: "op.carla", TitularTipo: tipoDe(pj.ID)},
			espera: "R5",
		},
	}

	for _, c := range cenarios {
		alertas, status, id, err := processar(ctx, d, cfg, c.tx, destino)
		if err != nil {
			return err
		}
		fmt.Printf("\n[%s] tx#%d valor=%s status=%s (esperado: %s)\n",
			c.nome, id, brl(c.tx.ValorCentavos), status, c.espera)
		imprimirAlertas(alertas)
	}

	// Cenário de velocidade (R4): rajada na mesma conta dentro da janela.
	fmt.Printf("\n[rajada de transacoes (R4)] origem conta#%d\n", pj.ID)
	for i := 0; i < cfg.Monitor.VelocidadeMax+2; i++ {
		tx := monitor.Transacao{ContaOrigemID: pj.ID, ValorCentavos: 600000, Tipo: "PIX",
			OcorridoEm:  agora.Add(time.Duration(i) * time.Minute),
			IniciadoPor: "op.carla", AprovadoPor: "ger.bruno", TitularTipo: tipoDe(pj.ID)}
		alertas, status, id, err := processar(ctx, d, cfg, tx, destino)
		if err != nil {
			return err
		}
		fmt.Printf("  tx#%d (%dª) status=%s\n", id, i+1, status)
		imprimirAlertas(alertas)
	}

	// Cenário de estruturação (R6): vários valores logo abaixo do limite.
	fmt.Printf("\n[estruturacao (R6)] origem conta#%d, limite=%s\n", pf.ID, brl(cfg.Monitor.LimiteRegulatorioCentavos))
	piso := int64(cfg.Monitor.MargemEstruturacao * float64(cfg.Monitor.LimiteRegulatorioCentavos))
	for i := 0; i < cfg.Monitor.MinOcorrenciasEstruturais+1; i++ {
		val := piso + int64(1000+i*500) // dentro da banda [piso, limite)
		tx := monitor.Transacao{ContaOrigemID: pf.ID, ValorCentavos: val, Tipo: "PIX",
			OcorridoEm:  agora.Add(time.Duration(i+30) * time.Minute),
			IniciadoPor: "op.ana", AprovadoPor: "ger.bruno", TitularTipo: tipoDe(pf.ID)}
		alertas, status, id, err := processar(ctx, d, cfg, tx, destino)
		if err != nil {
			return err
		}
		fmt.Printf("  tx#%d valor=%s status=%s\n", id, brl(val), status)
		imprimirAlertas(alertas)
	}

	return relatorioFinal(ctx, d)
}

// processar avalia uma transação no contexto atual do banco e a persiste.
func processar(ctx context.Context, d *db.DB, cfg config.Config, tx monitor.Transacao, destino int64) ([]monitor.Alerta, string, int64, error) {
	base, err := d.BaselineConta(ctx, tx.ContaOrigemID)
	if err != nil {
		return nil, "", 0, err
	}
	recentes, err := d.TransacoesRecentes(ctx, tx.ContaOrigemID, tx.OcorridoEm, cfg.Monitor.JanelaVelocidade)
	if err != nil {
		return nil, "", 0, err
	}
	alertas := monitor.Avaliar(tx, monitor.ContextoConta{Baseline: base, TransacoesRecentes: recentes}, cfg.Monitor)

	status := "APROVADA"
	if len(alertas) > 0 {
		status = "EM_ANALISE"
	}
	id, err := d.InserirTransacao(ctx, tx, destino, status, "simulacao")
	if err != nil {
		return nil, "", 0, err
	}
	for _, a := range alertas {
		if err := d.InserirAlerta(ctx, id, a); err != nil {
			return nil, "", 0, err
		}
	}
	return alertas, status, id, nil
}

func relatorioFinal(ctx context.Context, d *db.DB) error {
	conn := d.Conn()

	fmt.Println("\n== resumo de alertas por regra ==")
	rows, err := conn.QueryContext(ctx, `
		SELECT regra, nome_regra, severidade, COUNT(*)
		  FROM alertas GROUP BY regra, nome_regra, severidade ORDER BY regra`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var regra, nome, sev string
		var n int
		if err := rows.Scan(&regra, &nome, &sev, &n); err != nil {
			return err
		}
		fmt.Printf("  %-3s %-34s %-8s %d\n", regra, nome, sev, n)
	}

	total, err := audit.Total(ctx, conn)
	if err != nil {
		return err
	}
	fmt.Printf("\n== trilha de auditoria (SOX) ==\n  total de registros imutaveis: %d\n", total)
	resumo, err := audit.ResumoTrilha(ctx, conn)
	if err != nil {
		return err
	}
	for _, r := range resumo {
		fmt.Printf("  %-12s %-8s %d\n", r.Tabela, r.Operacao, r.Total)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func imprimirAlertas(alertas []monitor.Alerta) {
	for _, a := range alertas {
		fmt.Printf("    -> [%s|%s] %s: %s\n", a.Regra, a.Severidade, a.NomeRegra, a.Detalhe)
	}
}

func brl(c int64) string { return fmt.Sprintf("R$ %d,%02d", c/100, c%100) }

func outraConta(atual int64, todas []int64) int64 {
	for _, c := range todas {
		if c != atual {
			return c
		}
	}
	return atual
}

func outraContaInfo(atual int64, todas []db.ContaInfo) int64 {
	for _, c := range todas {
		if c.ID != atual {
			return c.ID
		}
	}
	return atual
}

func primeiraDoTipo(contas []db.ContaInfo, tt monitor.TitularTipo) db.ContaInfo {
	for _, c := range contas {
		if c.TitularTipo == tt {
			return c
		}
	}
	return contas[0]
}

// referenciaUtil retorna um instante determinístico em dia útil às 14h00,
// usado como "agora" da simulação para que o cenário normal não caia em R3.
func referenciaUtil(t time.Time) time.Time {
	for t.Weekday() == time.Saturday || t.Weekday() == time.Sunday {
		t = t.AddDate(0, 0, -1)
	}
	return time.Date(t.Year(), t.Month(), t.Day(), 14, 0, 0, 0, t.Location())
}

// diaUtilHorarioComercial empurra a data para um dia útil entre 9h e 17h.
func diaUtilHorarioComercial(t time.Time) time.Time {
	for t.Weekday() == time.Saturday || t.Weekday() == time.Sunday {
		t = t.AddDate(0, 0, -1)
	}
	return time.Date(t.Year(), t.Month(), t.Day(), 9+(t.Day()%8), 30, 0, 0, t.Location())
}

// --- upserts idempotentes -------------------------------------------------

func upsertBanco(ctx context.Context, conn *sql.DB, codigo, nome string) (int64, error) {
	var id int64
	err := conn.QueryRowContext(ctx, `
		INSERT INTO bancos (codigo, nome) VALUES ($1,$2)
		ON CONFLICT (codigo) DO UPDATE SET nome = EXCLUDED.nome
		RETURNING id`, codigo, nome).Scan(&id)
	return id, err
}

func upsertPF(ctx context.Context, conn *sql.DB, cpf, nome string) (int64, error) {
	var id int64
	err := conn.QueryRowContext(ctx, `
		INSERT INTO pessoas_fisicas (cpf, nome) VALUES ($1,$2)
		ON CONFLICT (cpf) DO UPDATE SET nome = EXCLUDED.nome
		RETURNING id`, cpf, nome).Scan(&id)
	return id, err
}

func upsertPJ(ctx context.Context, conn *sql.DB, cnpj, rs, nf string) (int64, error) {
	var id int64
	err := conn.QueryRowContext(ctx, `
		INSERT INTO pessoas_juridicas (cnpj, razao_social, nome_fantasia) VALUES ($1,$2,$3)
		ON CONFLICT (cnpj) DO UPDATE SET razao_social = EXCLUDED.razao_social
		RETURNING id`, cnpj, rs, nf).Scan(&id)
	return id, err
}

func upsertContaPF(ctx context.Context, conn *sql.DB, bancoID int64, ag, num string, pfID int64) (int64, error) {
	var id int64
	err := conn.QueryRowContext(ctx, `
		INSERT INTO contas (banco_id, agencia, numero, titular_tipo, pessoa_fisica_id)
		VALUES ($1,$2,$3,'PF',$4)
		ON CONFLICT (banco_id, agencia, numero) DO UPDATE SET numero = EXCLUDED.numero
		RETURNING id`, bancoID, ag, num, pfID).Scan(&id)
	return id, err
}

func upsertContaPJ(ctx context.Context, conn *sql.DB, bancoID int64, ag, num string, pjID int64) (int64, error) {
	var id int64
	err := conn.QueryRowContext(ctx, `
		INSERT INTO contas (banco_id, agencia, numero, titular_tipo, pessoa_juridica_id)
		VALUES ($1,$2,$3,'PJ',$4)
		ON CONFLICT (banco_id, agencia, numero) DO UPDATE SET numero = EXCLUDED.numero
		RETURNING id`, bancoID, ag, num, pjID).Scan(&id)
	return id, err
}

func checar(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "erro:", err)
		os.Exit(1)
	}
}
