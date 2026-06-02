package monitor

import (
	"fmt"
	"math"
	"time"
)

type TitularTipo string

const (
	PF TitularTipo = "PF"
	PJ TitularTipo = "PJ"
)

type Severidade string

const (
	Baixa   Severidade = "BAIXA"
	Media   Severidade = "MEDIA"
	Alta    Severidade = "ALTA"
	Critica Severidade = "CRITICA"
)

type Transacao struct {
	ID            int64
	ContaOrigemID int64
	ValorCentavos int64
	Tipo          string
	OcorridoEm    time.Time
	IniciadoPor   string
	AprovadoPor   string
	TitularTipo   TitularTipo
}

type Baseline struct {
	MediaCentavos  float64
	DesvioCentavos float64
	Amostras       int
}

type ContextoConta struct {
	Baseline           Baseline
	TransacoesRecentes []Transacao
}

type Alerta struct {
	Regra      string
	NomeRegra  string
	Severidade Severidade
	Detalhe    string
}

type Config struct {
	TetoValorCentavos int64

	ZScoreLimite        float64
	MinAmostrasBaseline int

	HoraInicioComercial int
	HoraFimComercial    int
	// Frequency
	VelocidadeMax    int
	JanelaVelocidade time.Duration

	SoDTetoCentavos int64

	LimiteRegulatorioCentavos int64
	MargemEstruturacao        float64
	MinOcorrenciasEstruturais int
}

func ConfigPadrao() Config {
	return Config{
		TetoValorCentavos:         5_000_000,
		ZScoreLimite:              3.0,
		MinAmostrasBaseline:       5,
		HoraInicioComercial:       6,
		HoraFimComercial:          22,
		VelocidadeMax:             5,
		JanelaVelocidade:          1 * time.Hour,
		SoDTetoCentavos:           1_000_000,
		LimiteRegulatorioCentavos: 1_000_000,
		MargemEstruturacao:        0.90,
		MinOcorrenciasEstruturais: 3,
	}
}

func Avaliar(t Transacao, ctx ContextoConta, cfg Config) []Alerta {
	var alertas []Alerta

	if a, ok := regraTeto(t, cfg); ok {
		alertas = append(alertas, a)
	}
	if a, ok := regraDesvio(t, ctx.Baseline, cfg); ok {
		alertas = append(alertas, a)
	}
	if a, ok := regraHorario(t, cfg); ok {
		alertas = append(alertas, a)
	}
	if a, ok := regraVelocidade(t, ctx, cfg); ok {
		alertas = append(alertas, a)
	}
	if a, ok := regraSegregacao(t, cfg); ok {
		alertas = append(alertas, a)
	}
	if a, ok := regraEstruturacao(t, ctx, cfg); ok {
		alertas = append(alertas, a)
	}
	return alertas
}

// R1 - valor acima do teto absoluto.
func regraTeto(t Transacao, cfg Config) (Alerta, bool) {
	if t.ValorCentavos <= cfg.TetoValorCentavos {
		return Alerta{}, false
	}
	sev := Alta
	if t.ValorCentavos > cfg.TetoValorCentavos*2 {
		sev = Critica
	}
	return Alerta{
		Regra:      "R1",
		NomeRegra:  "Valor acima do teto",
		Severidade: sev,
		Detalhe: fmt.Sprintf("Valor %s excede o teto de %s.",
			reais(t.ValorCentavos), reais(cfg.TetoValorCentavos)),
	}, true
}

// R2 - desvio estatístico em relação ao histórico (z-score).
func regraDesvio(t Transacao, b Baseline, cfg Config) (Alerta, bool) {
	if b.Amostras < cfg.MinAmostrasBaseline || b.DesvioCentavos <= 0 {
		return Alerta{}, false // histórico insuficiente para julgar
	}
	z := (float64(t.ValorCentavos) - b.MediaCentavos) / b.DesvioCentavos
	if z < cfg.ZScoreLimite {
		return Alerta{}, false
	}
	sev := Media
	switch {
	case z >= cfg.ZScoreLimite*2:
		sev = Critica
	case z >= cfg.ZScoreLimite*1.5:
		sev = Alta
	}
	return Alerta{
		Regra:      "R2",
		NomeRegra:  "Desvio estatistico do padrao",
		Severidade: sev,
		Detalhe: fmt.Sprintf("Valor %s está a %.1f desvios da média histórica (%s).",
			reais(t.ValorCentavos), z, reais(int64(b.MediaCentavos))),
	}, true
}

// R3 - transação fora do horário comercial ou em fim de semana.
func regraHorario(t Transacao, cfg Config) (Alerta, bool) {
	h := t.OcorridoEm.Hour()
	wd := t.OcorridoEm.Weekday()
	foraHorario := h < cfg.HoraInicioComercial || h >= cfg.HoraFimComercial
	fimDeSemana := wd == time.Saturday || wd == time.Sunday
	if !foraHorario && !fimDeSemana {
		return Alerta{}, false
	}
	// PJ operando fora do expediente é mais incomum que PF.
	sev := Baixa
	if t.TitularTipo == PJ {
		sev = Media
	}
	motivo := "fora do horário comercial"
	if fimDeSemana && !foraHorario {
		motivo = "em fim de semana"
	} else if fimDeSemana {
		motivo = "em fim de semana e fora do horário comercial"
	}
	return Alerta{
		Regra:      "R3",
		NomeRegra:  "Horario atipico",
		Severidade: sev,
		Detalhe: fmt.Sprintf("Transação %s (%s).",
			motivo, t.OcorridoEm.Format("2006-01-02 15:04")),
	}, true
}

// R4 - velocidade: muitas transações da mesma conta numa janela curta.
func regraVelocidade(t Transacao, ctx ContextoConta, cfg Config) (Alerta, bool) {
	limite := t.OcorridoEm.Add(-cfg.JanelaVelocidade)
	n := 0
	for _, r := range ctx.TransacoesRecentes {
		if r.ContaOrigemID == t.ContaOrigemID && !r.OcorridoEm.Before(limite) && !r.OcorridoEm.After(t.OcorridoEm) {
			n++
		}
	}
	n++ // inclui a transação em análise
	if n <= cfg.VelocidadeMax {
		return Alerta{}, false
	}
	return Alerta{
		Regra:      "R4",
		NomeRegra:  "Velocidade anormal",
		Severidade: Alta,
		Detalhe: fmt.Sprintf("%d transações em %s (limite %d).",
			n, cfg.JanelaVelocidade, cfg.VelocidadeMax),
	}, true
}

// R5 - segregação de funções (SOX): transações relevantes exigem que quem
// aprova seja diferente de quem inicia.
func regraSegregacao(t Transacao, cfg Config) (Alerta, bool) {
	if t.ValorCentavos < cfg.SoDTetoCentavos {
		return Alerta{}, false
	}
	if t.AprovadoPor != "" && t.AprovadoPor != t.IniciadoPor {
		return Alerta{}, false // controle satisfeito
	}
	detalhe := fmt.Sprintf("Transação de %s sem aprovador independente.", reais(t.ValorCentavos))
	if t.AprovadoPor == t.IniciadoPor && t.AprovadoPor != "" {
		detalhe = fmt.Sprintf("Mesmo operador iniciou e aprovou (%s) transação de %s.",
			t.IniciadoPor, reais(t.ValorCentavos))
	}
	return Alerta{
		Regra:      "R5",
		NomeRegra:  "Violacao de segregacao de funcoes",
		Severidade: Alta,
		Detalhe:    detalhe,
	}, true
}

// R6 - estruturação (smurfing): repetição de valores logo abaixo do limite
// regulatório, indicando tentativa de fracionar para evitar reporte.
func regraEstruturacao(t Transacao, ctx ContextoConta, cfg Config) (Alerta, bool) {
	piso := int64(cfg.MargemEstruturacao * float64(cfg.LimiteRegulatorioCentavos))
	teto := cfg.LimiteRegulatorioCentavos
	naBanda := func(v int64) bool { return v >= piso && v < teto }

	if !naBanda(t.ValorCentavos) {
		return Alerta{}, false
	}
	n := 1 // a transação atual
	for _, r := range ctx.TransacoesRecentes {
		if r.ContaOrigemID == t.ContaOrigemID && naBanda(r.ValorCentavos) {
			n++
		}
	}
	if n < cfg.MinOcorrenciasEstruturais {
		return Alerta{}, false
	}
	return Alerta{
		Regra:      "R6",
		NomeRegra:  "Possivel estruturacao",
		Severidade: Critica,
		Detalhe: fmt.Sprintf("%d transações entre %s e %s (logo abaixo do limite regulatório).",
			n, reais(piso), reais(teto)),
	}, true
}

// reais formata centavos como moeda para mensagens legíveis.
func reais(c int64) string {
	neg := ""
	if c < 0 {
		neg = "-"
		c = -c
	}
	return fmt.Sprintf("R$ %s%d,%02d", neg, c/100, int(math.Abs(float64(c%100))))
}
