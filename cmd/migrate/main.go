package main

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/go_lang_testing/banco-sox/internal/config"
	_ "github.com/lib/pq"
)

const dirMigracoes = "migrations"

func main() {
	if len(os.Args) < 2 {
		fmt.Println("uso: migrate [up|down|version]")
		os.Exit(1)
	}
	cmd := os.Args[1]

	cfg := config.Carregar()
	conn, err := sql.Open("postgres", cfg.DSN)
	checar(err)
	defer conn.Close()
	checar(conn.Ping())
	checar(garantirTabelaControle(conn))

	switch cmd {
	case "up":
		checar(up(conn))
	case "down":
		checar(down(conn))
	case "version":
		checar(version(conn))
	default:
		fmt.Printf("comando desconhecido: %s\n", cmd)
		os.Exit(1)
	}
}

func garantirTabelaControle(conn *sql.DB) error {
	_, err := conn.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			versao      TEXT PRIMARY KEY,
			aplicado_em TIMESTAMPTZ NOT NULL DEFAULT now()
		)`)
	return err
}

// versao extrai "0001" de "0001_schema.up.sql".
func versao(arquivo string) string {
	base := filepath.Base(arquivo)
	if i := strings.Index(base, "_"); i > 0 {
		return base[:i]
	}
	return base
}

func aplicadas(conn *sql.DB) (map[string]bool, error) {
	rows, err := conn.Query(`SELECT versao FROM schema_migrations`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := map[string]bool{}
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		m[v] = true
	}
	return m, rows.Err()
}

func arquivos(sufixo string) ([]string, error) {
	g, err := filepath.Glob(filepath.Join(dirMigracoes, "*"+sufixo))
	if err != nil {
		return nil, err
	}
	sort.Strings(g)
	return g, nil
}

func up(conn *sql.DB) error {
	feitas, err := aplicadas(conn)
	if err != nil {
		return err
	}
	ups, err := arquivos(".up.sql")
	if err != nil {
		return err
	}
	pendentes := 0
	for _, f := range ups {
		v := versao(f)
		if feitas[v] {
			continue
		}
		sqlTexto, err := os.ReadFile(f)
		if err != nil {
			return err
		}
		tx, err := conn.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(string(sqlTexto)); err != nil {
			tx.Rollback()
			return fmt.Errorf("migração %s: %w", v, err)
		}
		if _, err := tx.Exec(`INSERT INTO schema_migrations(versao) VALUES ($1)`, v); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		fmt.Printf("aplicada: %s\n", filepath.Base(f))
		pendentes++
	}
	if pendentes == 0 {
		fmt.Println("nenhuma migração pendente.")
	}
	return nil
}

func down(conn *sql.DB) error {
	feitas, err := aplicadas(conn)
	if err != nil {
		return err
	}
	if len(feitas) == 0 {
		fmt.Println("nada a reverter.")
		return nil
	}
	downs, err := arquivos(".down.sql")
	if err != nil {
		return err
	}
	// percorre em ordem decrescente, reverte a primeira aplicada que encontrar
	for i := len(downs) - 1; i >= 0; i-- {
		f := downs[i]
		v := versao(f)
		if !feitas[v] {
			continue
		}
		sqlTexto, err := os.ReadFile(f)
		if err != nil {
			return err
		}
		tx, err := conn.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(string(sqlTexto)); err != nil {
			tx.Rollback()
			return fmt.Errorf("reverter %s: %w", v, err)
		}
		if _, err := tx.Exec(`DELETE FROM schema_migrations WHERE versao = $1`, v); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		fmt.Printf("revertida: %s\n", filepath.Base(f))
		return nil
	}
	fmt.Println("nada a reverter.")
	return nil
}

func version(conn *sql.DB) error {
	rows, err := conn.Query(`SELECT versao, aplicado_em FROM schema_migrations ORDER BY versao`)
	if err != nil {
		return err
	}
	defer rows.Close()
	fmt.Println("migrações aplicadas:")
	algum := false
	for rows.Next() {
		var v, quando string
		if err := rows.Scan(&v, &quando); err != nil {
			return err
		}
		fmt.Printf("  %s  (%s)\n", v, quando)
		algum = true
	}
	if !algum {
		fmt.Println("  (nenhuma)")
	}
	return rows.Err()
}

func checar(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "erro:", err)
		os.Exit(1)
	}
}
