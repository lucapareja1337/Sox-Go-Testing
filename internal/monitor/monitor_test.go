package monitor

import (
	"testing"
	"time"
)

// horário comercial de referência usado nos testes (terça-feira, 14h)
func instanteComercial() time.Time {
	return time.Date(2025, 3, 11, 14, 0, 0, 0, time.UTC) // terça
}

func baselineNormal() Baseline {
	// média R$ 1.000,00, desvio R$ 100,00, amostra suficiente
	return Baseline{MediaCentavos: 100_000, DesvioCentavos: 10_000, Amostras: 30}
}

func txBase() Transacao {
	return Transacao{
		ID:            1,
		ContaOrigemID: 10,
		ValorCentavos: 100_000, // R$ 1.000,00 - dentro do padrão
		Tipo:          "PIX",
		OcorridoEm:    instanteComercial(),
		IniciadoPor:   "operador.ana",
		AprovadoPor:   "gerente.bruno",
		TitularTipo:   PF,
	}
}

func contemRegra(alertas []Alerta, regra string) bool {
	for _, a := range alertas {
		if a.Regra == regra {
			return true
		}
	}
	return false
}

func TestTransacaoNormalNaoGeraAlerta(t *testing.T) {
	cfg := ConfigPadrao()
	got := Avaliar(txBase(), ContextoConta{Baseline: baselineNormal()}, cfg)
	if len(got) != 0 {
		t.Fatalf("esperava 0 alertas para transação normal, obtive %d: %+v", len(got), got)
	}
}

func TestR1Teto(t *testing.T) {
	cfg := ConfigPadrao()
	tx := txBase()
	tx.ValorCentavos = 6_000_000 // R$ 60.000 > teto R$ 50.000
	got := Avaliar(tx, ContextoConta{Baseline: baselineNormal()}, cfg)
	if !contemRegra(got, "R1") {
		t.Fatalf("esperava R1, obtive %+v", got)
	}
}

func TestR1Critica(t *testing.T) {
	cfg := ConfigPadrao()
	tx := txBase()
	tx.ValorCentavos = 12_000_000 // > 2x teto
	for _, a := range Avaliar(tx, ContextoConta{Baseline: baselineNormal()}, cfg) {
		if a.Regra == "R1" && a.Severidade != Critica {
			t.Fatalf("esperava severidade CRITICA, obtive %s", a.Severidade)
		}
	}
}

func TestR2Desvio(t *testing.T) {
	cfg := ConfigPadrao()
	tx := txBase()
	// média 1.000 desvio 100 -> 1.500 = z 50 (bem acima de 3)
	tx.ValorCentavos = 150_000
	got := Avaliar(tx, ContextoConta{Baseline: baselineNormal()}, cfg)
	if !contemRegra(got, "R2") {
		t.Fatalf("esperava R2, obtive %+v", got)
	}
}

func TestR2HistoricoInsuficienteNaoDispara(t *testing.T) {
	cfg := ConfigPadrao()
	tx := txBase()
	tx.ValorCentavos = 150_000
	b := Baseline{MediaCentavos: 100_000, DesvioCentavos: 10_000, Amostras: 2} // < min
	got := Avaliar(tx, ContextoConta{Baseline: b}, cfg)
	if contemRegra(got, "R2") {
		t.Fatalf("não deveria disparar R2 com histórico insuficiente: %+v", got)
	}
}

func TestR3ForaHorario(t *testing.T) {
	cfg := ConfigPadrao()
	tx := txBase()
	tx.OcorridoEm = time.Date(2025, 3, 11, 3, 0, 0, 0, time.UTC) // 03h
	got := Avaliar(tx, ContextoConta{Baseline: baselineNormal()}, cfg)
	if !contemRegra(got, "R3") {
		t.Fatalf("esperava R3 (madrugada), obtive %+v", got)
	}
}

func TestR3FimDeSemana(t *testing.T) {
	cfg := ConfigPadrao()
	tx := txBase()
	tx.OcorridoEm = time.Date(2025, 3, 15, 14, 0, 0, 0, time.UTC) // sábado 14h
	got := Avaliar(tx, ContextoConta{Baseline: baselineNormal()}, cfg)
	if !contemRegra(got, "R3") {
		t.Fatalf("esperava R3 (fim de semana), obtive %+v", got)
	}
}

func TestR4Velocidade(t *testing.T) {
	cfg := ConfigPadrao()
	tx := txBase()
	base := tx.OcorridoEm
	var recentes []Transacao
	for i := 0; i < cfg.VelocidadeMax; i++ { // já no limite; a atual estoura
		recentes = append(recentes, Transacao{
			ContaOrigemID: tx.ContaOrigemID,
			ValorCentavos: 100_000,
			OcorridoEm:    base.Add(-time.Duration(i+1) * time.Minute),
		})
	}
	got := Avaliar(tx, ContextoConta{Baseline: baselineNormal(), TransacoesRecentes: recentes}, cfg)
	if !contemRegra(got, "R4") {
		t.Fatalf("esperava R4, obtive %+v", got)
	}
}

func TestR5SegregacaoSemAprovador(t *testing.T) {
	cfg := ConfigPadrao()
	tx := txBase()
	tx.ValorCentavos = 2_000_000 // acima do teto de SoD
	tx.AprovadoPor = ""          // sem aprovador
	got := Avaliar(tx, ContextoConta{Baseline: baselineNormal()}, cfg)
	if !contemRegra(got, "R5") {
		t.Fatalf("esperava R5 (sem aprovador), obtive %+v", got)
	}
}

func TestR5MesmoOperadorIniciaEAprova(t *testing.T) {
	cfg := ConfigPadrao()
	tx := txBase()
	tx.ValorCentavos = 2_000_000
	tx.IniciadoPor = "operador.ana"
	tx.AprovadoPor = "operador.ana" // mesma pessoa
	got := Avaliar(tx, ContextoConta{Baseline: baselineNormal()}, cfg)
	if !contemRegra(got, "R5") {
		t.Fatalf("esperava R5 (mesmo operador), obtive %+v", got)
	}
}

func TestR5SegregacaoSatisfeitaNaoDispara(t *testing.T) {
	cfg := ConfigPadrao()
	tx := txBase()
	tx.ValorCentavos = 2_000_000
	tx.IniciadoPor = "operador.ana"
	tx.AprovadoPor = "gerente.bruno"
	got := Avaliar(tx, ContextoConta{Baseline: baselineNormal()}, cfg)
	if contemRegra(got, "R5") {
		t.Fatalf("não deveria disparar R5 com aprovador independente: %+v", got)
	}
}

func TestR6Estruturacao(t *testing.T) {
	cfg := ConfigPadrao()
	// limite R$ 10.000; banda [R$ 9.000, R$ 10.000)
	tx := txBase()
	tx.ValorCentavos = 950_000 // R$ 9.500
	base := tx.OcorridoEm
	recentes := []Transacao{
		{ContaOrigemID: tx.ContaOrigemID, ValorCentavos: 920_000, OcorridoEm: base.Add(-10 * time.Minute)},
		{ContaOrigemID: tx.ContaOrigemID, ValorCentavos: 980_000, OcorridoEm: base.Add(-20 * time.Minute)},
	}
	got := Avaliar(tx, ContextoConta{Baseline: baselineNormal(), TransacoesRecentes: recentes}, cfg)
	if !contemRegra(got, "R6") {
		t.Fatalf("esperava R6 (estruturação), obtive %+v", got)
	}
}

func TestReaisFormatacao(t *testing.T) {
	casos := map[int64]string{
		0:      "R$ 0,00",
		5:      "R$ 0,05",
		100:    "R$ 1,00",
		123456: "R$ 1234,56",
		-2550:  "R$ -25,50",
	}
	for cent, want := range casos {
		if got := reais(cent); got != want {
			t.Errorf("reais(%d) = %q, esperava %q", cent, got, want)
		}
	}
}
