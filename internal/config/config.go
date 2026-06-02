// Package config centraliza a leitura de configuração via variáveis de ambiente.
// Em um cenário SOX, parametrizar limiares por configuração versionada facilita
// a auditoria de "qual controle estava ativo em qual momento".
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/go_lang_testing/banco-sox/internal/monitor"
)

// Config agrega a configuração de banco e do motor de regras.
type Config struct {
	DSN     string
	Monitor monitor.Config
}

// Carregar lê as variáveis de ambiente, aplicando padrões seguros.
func Carregar() Config {
	host := env("PGHOST", "localhost")
	port := env("PGPORT", "5432")
	user := env("PGUSER", "banco")
	pass := env("PGPASSWORD", "banco")
	name := env("PGDATABASE", "banco_sox")
	sslmode := env("PGSSLMODE", "disable")

	dsn := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		host, port, user, pass, name, sslmode,
	)

	mc := monitor.ConfigPadrao()
	mc.TetoValorCentavos = envInt64("SOX_TETO_CENTAVOS", mc.TetoValorCentavos)
	mc.ZScoreLimite = envFloat("SOX_ZSCORE", mc.ZScoreLimite)
	mc.SoDTetoCentavos = envInt64("SOX_SOD_TETO_CENTAVOS", mc.SoDTetoCentavos)
	mc.LimiteRegulatorioCentavos = envInt64("SOX_LIMITE_REGULATORIO_CENTAVOS", mc.LimiteRegulatorioCentavos)
	if v := os.Getenv("SOX_JANELA_VELOCIDADE"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			mc.JanelaVelocidade = d
		}
	}

	return Config{DSN: dsn, Monitor: mc}
}

func env(chave, padrao string) string {
	if v := os.Getenv(chave); v != "" {
		return v
	}
	return padrao
}

func envInt64(chave string, padrao int64) int64 {
	if v := os.Getenv(chave); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return n
		}
	}
	return padrao
}

func envFloat(chave string, padrao float64) float64 {
	if v := os.Getenv(chave); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return padrao
}
