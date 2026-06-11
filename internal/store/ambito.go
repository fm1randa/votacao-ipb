package store

// Multi-âmbito (ADR-0009): o motor de eleição é um só (Art. 90 ≡ Art. 91 d–g);
// âmbito e sociedade são configuração — definem o preset de cargos, a regra de
// quórum, os limites de idade para concorrer e o vocabulário da UI (SPEC §10).

import (
	"errors"
	"fmt"
	"time"
)

var (
	errInvalidAmbito  = errors.New("âmbito ou sociedade inválidos")
	errUCPSemNacional = errors.New("a UCP não possui Confederação Nacional (GTSI, Específica UCP)")
)

// Âmbitos (nível da entidade cuja Diretoria se elege).
const (
	AmbitoLocal     = "local"
	AmbitoFederacao = "federacao"
	AmbitoSinodal   = "sinodal"
	AmbitoNacional  = "nacional"
)

// Sociedades internas do GTSI.
var Sociedades = []string{"UMP", "UPA", "UPH", "SAF", "UCP"}

func ValidAmbito(a string) bool {
	switch a {
	case AmbitoLocal, AmbitoFederacao, AmbitoSinodal, AmbitoNacional:
		return true
	}
	return false
}

func ValidSociedade(s string) bool {
	for _, x := range Sociedades {
		if x == s {
			return true
		}
	}
	return false
}

// ValidateAmbitoSociedade valida a combinação. A UCP não possui Confederação
// Nacional ("pela sua excepcionalidade" — GTSI, Específica UCP).
func ValidateAmbitoSociedade(ambito, sociedade string) error {
	if !ValidAmbito(ambito) || !ValidSociedade(sociedade) {
		return errInvalidAmbito
	}
	if ambito == AmbitoNacional && sociedade == "UCP" {
		return errUCPSemNacional
	}
	return nil
}

// Critério de subunidades (Federações) no quórum do Congresso Nacional — varia
// por sociedade no GTSI vigente:
//   - UMP (Art. 49c): mais da metade das Sinodais E ≥ 1/3 das Federações;
//   - UPH (Art. 134) e SAF (Art. 91): mais da metade das Sinodais E das Federações;
//   - UPA (Art. 62): só mais da metade das Sinodais.
const (
	SubRegraTerco  = "terco"  // ≥ 1/3 das Federações (UMP)
	SubRegraMetade = "metade" // > 1/2 das Federações (UPH, SAF)
	SubRegraNada   = ""       // sem critério de Federações (UPA)
)

// NacionalSubRegra devolve o critério de Federações do quórum Nacional.
func NacionalSubRegra(sociedade string) string {
	switch sociedade {
	case "UMP":
		return SubRegraTerco
	case "UPH", "SAF":
		return SubRegraMetade
	}
	return SubRegraNada
}

// ---------------------------------------------------------------------------
// Presets de cargos (SPEC §10.2)
// ---------------------------------------------------------------------------

// Roles identificam o papel do cargo independentemente do nome exibido
// (gênero da sociedade, regiões da Nacional).
const (
	RolePresidente  = "presidente"
	RoleVice        = "vice"
	RoleSecExec     = "sec_executivo"
	RolePrimeiroSec = "primeiro_sec"
	RoleSegundoSec  = "segundo_sec"
	RoleTesoureiro  = "tesoureiro"
	// Vices regionais da Nacional (Art. 26b): cargos separados, um por região.
	roleViceRegional = "vice_" // prefixo: vice_norte, vice_sudeste_sul, ...
)

// PositionPreset descreve um cargo do preset de um âmbito×sociedade.
type PositionPreset struct {
	Role     string
	Nome     string
	Optional bool // pode ser desativado nas configurações
}

// presetNames: nomes por sociedade — a SAF usa títulos femininos.
func presetNames(sociedade string) map[string]string {
	if sociedade == "SAF" {
		return map[string]string{
			RolePresidente:  "Presidente",
			RoleVice:        "Vice-presidente",
			RoleSecExec:     "Secretária Executiva",
			RolePrimeiroSec: "1ª Secretária",
			RoleSegundoSec:  "2ª Secretária",
			RoleTesoureiro:  "Tesoureira",
		}
	}
	return map[string]string{
		RolePresidente:  "Presidente",
		RoleVice:        "Vice-presidente",
		RoleSecExec:     "Secretário Executivo",
		RolePrimeiroSec: "1º Secretário",
		RoleSegundoSec:  "2º Secretário",
		RoleTesoureiro:  "Tesoureiro",
	}
}

// regioesNacional: vices regionais da Nacional — 5 regiões; na SAF são 6
// (Sudeste conta Norte e Sul — Específica SAF).
func regioesNacional(sociedade string) [][2]string {
	if sociedade == "SAF" {
		return [][2]string{
			{"vice_norte", "Norte"}, {"vice_nordeste", "Nordeste"},
			{"vice_centro_oeste", "Centro-Oeste"},
			{"vice_sudeste_norte", "Sudeste Norte"}, {"vice_sudeste_sul", "Sudeste Sul"},
			{"vice_sul", "Sul"},
		}
	}
	return [][2]string{
		{"vice_norte", "Norte"}, {"vice_nordeste", "Nordeste"},
		{"vice_centro_oeste", "Centro-Oeste"}, {"vice_sudeste", "Sudeste"},
		{"vice_sul", "Sul"},
	}
}

// PresetPositions devolve os cargos do âmbito×sociedade, na ordem de eleição.
//   - Local (Art. 13): sem Secretário Executivo; redução expressa => Vice e 2º
//     Secretário opcionais (mínimo: Presidente, Secretário e Tesoureiro).
//   - Federação/Sinodal (Art. 26a): completos; Vice/Sec.Exec/2º Sec opcionais
//     por analogia (aviso normativo — SPEC §3.5).
//   - Nacional (Art. 26b): prescritivo (nada opcional), vices regionais como
//     cargos separados.
func PresetPositions(ambito, sociedade string) []PositionPreset {
	n := presetNames(sociedade)
	// UCP tem Secretário ÚNICO (Específica UCP: Art. 9º local; Art. 28
	// federados) — não há 1º/2º. O role continua primeiro_sec.
	if sociedade == "UCP" {
		sec := secretarioSolo(n[RolePrimeiroSec])
		if ambito == AmbitoLocal {
			return []PositionPreset{
				{RolePresidente, n[RolePresidente], false},
				{RoleVice, n[RoleVice], true},
				{RolePrimeiroSec, sec, false},
				{RoleTesoureiro, n[RoleTesoureiro], false},
			}
		}
		return []PositionPreset{ // federação/sinodal (nacional não existe)
			{RolePresidente, n[RolePresidente], false},
			{RoleVice, n[RoleVice], true},
			{RoleSecExec, n[RoleSecExec], true},
			{RolePrimeiroSec, sec, false},
			{RoleTesoureiro, n[RoleTesoureiro], false},
		}
	}
	switch ambito {
	case AmbitoLocal:
		return []PositionPreset{
			{RolePresidente, n[RolePresidente], false},
			{RoleVice, n[RoleVice], true},
			{RolePrimeiroSec, n[RolePrimeiroSec], false},
			{RoleSegundoSec, n[RoleSegundoSec], true},
			{RoleTesoureiro, n[RoleTesoureiro], false},
		}
	case AmbitoNacional:
		out := []PositionPreset{{RolePresidente, n[RolePresidente], false}}
		for _, r := range regioesNacional(sociedade) {
			out = append(out, PositionPreset{r[0], n[RoleVice] + " " + r[1], false})
		}
		return append(out,
			PositionPreset{RoleSecExec, n[RoleSecExec], false},
			PositionPreset{RolePrimeiroSec, n[RolePrimeiroSec], false},
			PositionPreset{RoleSegundoSec, n[RoleSegundoSec], false},
			PositionPreset{RoleTesoureiro, n[RoleTesoureiro], false},
		)
	default: // federacao, sinodal — Art. 26a
		return []PositionPreset{
			{RolePresidente, n[RolePresidente], false},
			{RoleVice, n[RoleVice], true},
			{RoleSecExec, n[RoleSecExec], true},
			{RolePrimeiroSec, n[RolePrimeiroSec], false},
			{RoleSegundoSec, n[RoleSegundoSec], true},
			{RoleTesoureiro, n[RoleTesoureiro], false},
		}
	}
}

// OptionalRole diz se um cargo (role) é desativável no âmbito.
func OptionalRole(ambito, sociedade, role string) bool {
	for _, p := range PresetPositions(ambito, sociedade) {
		if p.Role == role {
			return p.Optional
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Limites de idade para concorrer (Parte Comum Art. 4º §3–4)
// ---------------------------------------------------------------------------

// AgeMax devolve a idade máxima para SER VOTADO no âmbito (0 = sem limite).
// Vale só para Sinodal/Nacional de UMP e UPA; votar não tem limite.
func AgeMax(ambito, sociedade string) int {
	switch ambito {
	case AmbitoSinodal:
		switch sociedade {
		case "UMP":
			return 33
		case "UPA":
			return 17
		}
	case AmbitoNacional:
		switch sociedade {
		case "UMP":
			return 32
		case "UPA":
			return 16
		}
	}
	return 0
}

// ageCutoff devolve a data de nascimento limite: nascido NESTA data ou antes
// já excede a idade máxima (faz aniversário de max+1 hoje ou antes). Quem tem
// nascimento vazio (legado) não é bloqueado — dado ausente não pode ser gate.
func ageCutoff(maxAge int) string {
	return time.Now().AddDate(-(maxAge + 1), 0, 0).Format("2006-01-02")
}

// ageEligibleSQL devolve uma condição SQL (sobre o alias `e`) que exclui quem
// excede a idade máxima do âmbito. Vazio quando não há limite.
func ageEligibleSQL(ambito, sociedade string) string {
	max := AgeMax(ambito, sociedade)
	if max == 0 {
		return ""
	}
	return fmt.Sprintf(" AND (e.nascimento IS NULL OR e.nascimento = '' OR e.nascimento > '%s')", ageCutoff(max))
}

// ageEligible aplica o mesmo critério em Go (validação do voto).
func ageEligible(ambito, sociedade, nascimento string) bool {
	max := AgeMax(ambito, sociedade)
	if max == 0 || nascimento == "" {
		return true
	}
	return nascimento > ageCutoff(max)
}
