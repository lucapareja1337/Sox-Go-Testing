package model

import "time"

type Banco struct {
	ID     int64
	Codigo string
	Nome   string
}

type PessoaFisica struct {
	ID   int64
	CPF  string
	Nome string
}

type PessoaJuridica struct {
	ID           int64
	CNPJ         string
	RazaoSocial  string
	NomeFantasia string
}

type Conta struct {
	ID             int64
	BancoID        int64
	Agencia        string
	Numero         string
	TitularTipo    string
	PessoaFisica   *int64
	PessoaJuridica *int64
	SaldoCentavos  int64
}

type Transacao struct {
	ID             int64
	ContaOrigemID  int64
	ContaDestinoID int64
	ValorCentavos  int64
	Tipo           string
	Status         string
	IniciadoPor    string
	AprovadoPor    string
	Descricao      string
	OcorridoEm     time.Time
}
