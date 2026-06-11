package store

import "strings"

// Atribuições por cargo, transcritas na íntegra do GTSI vigente (Parte Comum +
// Específicas chanceladas pela SC-2018). Alimentam o botão "Ler atribuições"
// na Mesa e na cédula — o texto que o presidente lê ao plenário antes do voto.
//
// A organização varia por Específica:
//   - UMP: um conjunto local (Arts. 15–19) e UM só para os federados
//     (Arts. 28–33, "Federação ou Confederação");
//   - UPH e UPA: um conjunto por âmbito;
//   - SAF: local, um conjunto para Federação E Sinodal (Arts. 46–54) e um
//     próprio da Nacional (Arts. 75–80);
//   - UCP: local e Federação; a Sinodal reusa os artigos da Federação com
//     termos substituídos (Específica UCP, Art. 51). Não há Nacional.

type Atribuicao struct {
	Ref   string   // ex.: "Específica UMP, Art. 28"
	Itens []string // alíneas, na ordem e grafia do texto
	Nota  string   // parágrafo único/observação, quando houver
}

func at(ref string, itens ...string) Atribuicao { return Atribuicao{Ref: ref, Itens: itens} }

// atribSegment resolve qual conjunto de artigos vale para o âmbito na Específica.
func atribSegment(ambito, sociedade string) string {
	switch sociedade {
	case "UMP":
		if ambito == AmbitoLocal {
			return "local"
		}
		return "federados"
	case "SAF":
		switch ambito {
		case AmbitoLocal:
			return "local"
		case AmbitoNacional:
			return "nacional"
		default:
			return "federacao" // Federação e Sinodal: Arts. 46–54
		}
	case "UCP":
		if ambito == AmbitoLocal {
			return "local"
		}
		return "federacao" // Sinodal: mesmos artigos (Art. 51)
	default: // UPH, UPA
		return ambito
	}
}

// AtribuicoesCargo devolve o texto do GTSI para o cargo no âmbito × sociedade.
// Vices regionais da Nacional usam o artigo dos Vice-Presidentes. ok=false
// quando não há texto para o cargo.
func AtribuicoesCargo(ambito, sociedade, role string) (Atribuicao, bool) {
	if strings.HasPrefix(role, roleViceRegional) {
		role = RoleVice
	}
	seg, ok := atribuicoes[sociedade]
	if !ok {
		return Atribuicao{}, false
	}
	a, ok := seg[atribSegment(ambito, sociedade)][role]
	if !ok {
		return Atribuicao{}, false
	}
	// Notas que valem só num âmbito específico do conjunto compartilhado.
	if sociedade == "UMP" && ambito == AmbitoNacional && role == RoleVice {
		a.Nota = "Parágrafo único — No caso da Confederação Nacional de Mocidade (CNM), " +
			"os vice-presidentes regionais também terão como atribuição zelar para que os " +
			"objetivos, planos e realizações da CNM sejam conhecidos e cumpridos em suas " +
			"respectivas regiões."
	}
	if sociedade == "UCP" && ambito == AmbitoSinodal {
		a.Nota = strings.TrimSpace(a.Nota + " Na Confederação Sinodal aplicam-se os mesmos artigos da " +
			"Federação, substituindo-se os termos (UCP por Federação, Federação por Confederação, " +
			"Presbitério por Sínodo — Específica UCP, Art. 51).")
	}
	return a, true
}

var atribuicoes = map[string]map[string]map[string]Atribuicao{
	"UMP": {
		"local": {
			RolePresidente: at("Específica UMP, Art. 15",
				"a) convocar e presidir todas as reuniões da UMP;",
				"b) acompanhar as atividades da UMP, estimulando e orientando a todos na maneira de alcançar os planos aprovados;",
				"c) representar a UMP onde se fizer necessário;",
				"d) dar voto de minerva quando necessário;",
				"e) apresentar relatório de atividades da UMP para a Assembleia Geral da igreja, ou para o seu Conselho, quando for o caso, e para a Federação;"),
			RoleVice: at("Específica UMP, Art. 16",
				"a) cooperar com o Presidente no exercício de suas funções;",
				"b) substituir o Presidente em suas faltas e impedimentos."),
			RolePrimeiroSec: at("Específica UMP, Art. 17",
				"a) zelar pelo registro das reuniões e atividades;",
				"b) zelar pela guarda e integridade dos documentos da sociedade, quando se fizer necessário."),
			RoleSegundoSec: at("Específica UMP, Art. 18",
				"a) substituir o Primeiro Secretário em suas faltas e impedimentos eventuais;",
				"b) exercer as funções de relações públicas."),
			RoleTesoureiro: at("Específica UMP, Art. 19",
				"a) receber as verbas da UMP e recolher a anuidade ou contribuição individual, repassando o valor total correspondente para a Federação;",
				"b) efetuar pagamentos;",
				"c) efetuar registros de movimentação financeira e prestar contas sempre que solicitado."),
		},
		"federados": {
			RolePresidente: at("Específica UMP, Art. 28",
				"a) convocar, por meio do Secretário Executivo, e presidir as reuniões da Diretoria, da Comissão Executiva e do Congresso;",
				"b) apresentar relatório das atividades da Federação ou Confederação, enviando cópia deste ao respectivo Secretário e à sua instância superior;",
				"c) representar a Federação ou Confederação onde se fizer necessário;",
				"d) dar voto de “Minerva” no caso de empate na votação de matérias e eleições;",
				"e) assinar, com o Tesoureiro, ordens de pagamento e balancetes da Federação, da Confederação Sinodal ou da Confederação Nacional."),
			RoleVice: at("Específica UMP, Art. 29",
				"a) cooperar com o Presidente no exercício de suas funções;",
				"b) substituir o Presidente em suas faltas e impedimentos;",
				"c) cooperar com o secretário executivo no exercício de suas funções."),
			RoleSecExec: at("Específica UMP, Art. 30",
				"a) zelar pela pronta e fiel execução das resoluções emanadas dos Congressos e da Diretoria;",
				"b) receber os relatórios das Comissões nomeadas em Congresso e os demais papéis, conservando-os em ordem;",
				"c) organizar e manter em dia o arquivo;",
				"d) assinar e enviar, por ordem do Presidente, toda a correspondência;",
				"e) substituir o Presidente em suas faltas e impedimentos, estando ausente o Vice-Presidente;",
				"f) convocar, por ordem do Presidente, todas as reuniões;",
				"g) promover a ampla divulgação das atividades e informativos pertinentes da Federação ou Confederações."),
			RolePrimeiroSec: at("Específica UMP, Art. 31",
				"a) redigir os registros das reuniões;",
				"b) substituir o Secretário Executivo em suas faltas e impedimentos eventuais."),
			RoleSegundoSec: at("Específica UMP, Art. 32",
				"a) substituir o Primeiro Secretário em suas faltas e impedimentos eventuais;",
				"b) exercer as funções de relações públicas;",
				"c) organizar os protocolos de papéis que forem apresentados ao Congresso e encaminhá-los ao Secretário Executivo após o encerramento do congresso."),
			RoleTesoureiro: at("Específica UMP, Art. 33",
				"a) receber a anuidade por contribuição individual e repassar os devidos percentuais para as instâncias superiores;",
				"b) receber verbas e doações;",
				"c) organizar e manter em dia os livros próprios da tesouraria;",
				"d) apresentar relatório ao plenário do Congresso;",
				"e) elaborar o orçamento anual e apresentá-lo à Diretoria e ao plenário do Congresso para aprovação;",
				"f) assinar, com o Presidente, os cheques, ordens de pagamento e balancetes."),
		},
	},

	"UPH": {
		AmbitoLocal: {
			RolePresidente: at("Específica UPH, Art. 19",
				"a) convocar todas as reuniões: da Diretoria, da Comissão Executiva e plenárias;",
				"b) elaborar planos, junto com a Diretoria, e apresentá-los à Comissão Executiva e à plenária;",
				"c) acompanhar as atividades da UPH, estimulando e orientando a todos na maneira de alcançar os planos aprovados;",
				"d) representar a UPH onde se fizer necessário;",
				"e) presidir as reuniões da Diretoria, da Comissão Executiva e as plenárias;",
				"f) pôr em discussão as propostas apresentadas, esclarecendo com brevidade os assuntos a serem votados;",
				"g) suspender a sessão por proposta devidamente apoiada e votada;",
				"h) receber e apresentar quaisquer visitantes ou representantes de organizações congêneres;",
				"i) dar voto de Minerva no caso de empate na votação de matérias."),
			RoleVice: at("Específica UPH, Art. 20",
				"a) cooperar com o presidente no exercício de suas funções;",
				"b) substituir o presidente em suas faltas e impedimentos eventuais."),
			RolePrimeiroSec: at("Específica UPH, Art. 21",
				"a) redigir as atas da plenária, da Diretoria e da Comissão Executiva;",
				"b) substituir o presidente, no impedimento do vice-presidente."),
			RoleSegundoSec: at("Específica UPH, Art. 22",
				"a) encarregar-se da correspondência, dando ciência dela à UPH;",
				"b) cuidar do arquivo, do fichário e do rol de sócios;",
				"c) substituir o primeiro secretário em suas faltas e impedimentos."),
			RoleTesoureiro: at("Específica UPH, Art. 23",
				"a) receber verbas, taxa per capita e doações, escriturando-as devidamente em livro próprio;",
				"b) efetuar pagamentos conforme resoluções da plenária ou da Diretoria, neste último caso sempre ad referendum da próxima plenária;",
				"c) efetuar o pagamento da taxa per capita à Federação;",
				"d) elaborar o plano financeiro anual e apresentá-lo para a aprovação da plenária;",
				"e) apresentar balancete mensal à plenária, e relatório anual ao Conselho da igreja."),
		},
		AmbitoFederacao: {
			RolePresidente: at("Específica UPH, Art. 59",
				"a) convocar (através do Secretário-Executivo) e presidir as reuniões da Diretoria, da Comissão Executiva e do Congresso;",
				"b) elaborar planos e submetê-los à aprovação da Diretoria da Federação e do Secretário Presbiterial;",
				"c) apresentar relatório das atividades da Federação, enviando cópia deste ao Secretário Presbiterial e à Confederação Sinodal;",
				"d) representar a Federação onde se fizer necessário;",
				"e) dar voto de “Minerva” no caso de empate na votação de matérias e eleições;",
				"f) assinar, com o Tesoureiro, ordens de pagamento e balancetes da Federação."),
			RoleVice: at("Específica UPH, Art. 60",
				"a) cooperar com o Presidente no exercício de suas funções;",
				"b) substituir o Presidente em suas faltas e impedimentos."),
			RoleSecExec: at("Específica UPH, Art. 61",
				"a) zelar pela pronta e fiel execução das resoluções emanadas dos Congressos e da Diretoria;",
				"b) receber os relatórios das Comissões nomeadas em Congresso e os demais papéis, conservando-os em ordem;",
				"c) organizar e manter em dia o arquivo da Federação;",
				"d) assinar e enviar, por ordem do Presidente, toda a correspondência da Federação;",
				"e) substituir o Presidente em suas faltas e impedimentos, estando ausente o Vice-Presidente;",
				"f) convocar, por ordem do Presidente, todas as reuniões da Federação;",
				"g) elaborar e publicar Boletins da Federação."),
			RolePrimeiroSec: at("Específica UPH, Art. 62",
				"a) redigir e lavrar as atas das reuniões;",
				"b) substituir o Secretário-Executivo em suas faltas e impedimentos eventuais."),
			RoleSegundoSec: at("Específica UPH, Art. 63",
				"a) substituir o Primeiro Secretário em suas faltas e impedimentos eventuais;",
				"b) exercer as funções de relações públicas."),
			RoleTesoureiro: at("Específica UPH, Art. 64",
				"a) receber o percentual da taxa per capita correspondente das UPHs locais;",
				"b) receber verbas e doações;",
				"c) organizar e manter em dia os livros próprios da tesouraria;",
				"d) apresentar relatório anual ao plenário do Congresso e ao Presbitério, através do Secretário Presbiterial, juntamente com o relatório do Presidente;",
				"e) efetuar pagamento da taxa per capita à Confederação Sinodal;",
				"f) elaborar o orçamento anual e apresentá-lo à Diretoria e ao plenário do Congresso para aprovação;",
				"g) assinar, com o Presidente, os cheques, ordens de pagamento e balancetes da Federação."),
		},
		AmbitoSinodal: {
			RolePresidente: at("Específica UPH, Art. 90",
				"a) convocar (através do Secretário Executivo) e presidir as reuniões da Diretoria, da Comissão Executiva e dos Congressos;",
				"b) elaborar planos e submetê-los à aprovação da Diretoria da Confederação Sinodal e do Secretário Sinodal;",
				"c) apresentar relatórios das atividades da Confederação Sinodal ao congresso bienal, com cópias ao Secretário Sinodal e à Confederação Nacional;",
				"d) representar a Confederação onde se fizer necessário;",
				"e) dar voto de Minerva no caso de empate na votação de matérias e eleições;",
				"f) assinar, com o Tesoureiro, os cheques, ordens de pagamento e balancetes da Confederação Sinodal."),
			RoleVice: at("Específica UPH, Art. 91",
				"a) cooperar com o Presidente no exercício de suas funções;",
				"b) substituir o Presidente em suas faltas e impedimentos eventuais."),
			RoleSecExec: at("Específica UPH, Art. 92",
				"a) zelar pela pronta e fiel execução das resoluções emanadas dos Congressos e Diretorias;",
				"b) receber os relatórios das comissões nomeadas em Congresso e os demais papéis, conservando-os em ordem;",
				"c) organizar e manter em dia o arquivo da Confederação;",
				"d) assinar e arquivar, por ordem do Presidente, toda a correspondência da Confederação Sinodal;",
				"e) substituir o Presidente em suas faltas e impedimentos eventuais, estando ausente o Vice-Presidente;",
				"f) convocar, por ordem do Presidente, todas as reuniões da Diretoria, Comissão Executiva e Congresso Sinodal;",
				"g) elaborar e publicar Boletim da Confederação Sinodal."),
			RolePrimeiroSec: at("Específica UPH, Art. 93",
				"a) redigir e lavrar as atas das reuniões;",
				"b) substituir o Secretário Executivo em suas faltas e impedimentos eventuais."),
			RoleSegundoSec: at("Específica UPH, Art. 94",
				"a) substituir o primeiro-secretário em suas faltas e impedimentos eventuais;",
				"b) exercer as funções de relações públicas."),
			RoleTesoureiro: at("Específica UPH, Art. 95",
				"a) receber a taxa per capita;",
				"b) receber verbas e doações;",
				"c) organizar e manter em dia os livros próprios da tesouraria;",
				"d) apresentar relatórios trimestrais à Diretoria, e um bienal ao Congresso e ao Sínodo, neste caso através do Secretário Sinodal;",
				"e) efetuar pagamento da taxa per capita à Confederação Nacional;",
				"f) elaborar o plano anual orçamentário e apresentá-lo à Diretoria para aprovação;",
				"g) assinar, com o Presidente, os cheques, ordens de pagamento e balancetes da Confederação Sinodal."),
		},
		AmbitoNacional: {
			RolePresidente: at("Específica UPH, Art. 118",
				"a) convocar (através do Secretário Executivo) e presidir as reuniões da Diretoria, da Comissão Executiva e dos Congressos;",
				"b) elaborar planos e submetê-los à aprovação da Diretoria da Confederação Nacional e do Secretário Nacional;",
				"c) apresentar relatórios das atividades da Confederação Nacional ao Congresso e, através do Secretário Nacional, ao Supremo Concílio;",
				"d) dar voto de “Minerva” em casos de empate, na votação de matérias e eleições;",
				"e) assinar, com o Tesoureiro, os cheques, ordens de pagamentos e balancetes da Confederação Nacional."),
			RoleVice: at("Específica UPH, Art. 119",
				"a) cooperar com o Presidente no exercício de suas funções;",
				"b) substituir o Presidente em suas faltas e impedimentos eventuais, por ordem de idade, a começar do mais velho."),
			RoleSecExec: at("Específica UPH, Art. 120",
				"a) assinar e enviar, por ordem do Presidente, toda a correspondência oficial da Confederação Nacional;",
				"b) organizar e manter em dia o arquivo da Confederação Nacional;",
				"c) zelar pela pronta e fiel execução das resoluções emanadas do Congresso Nacional, da Comissão Executiva e da Diretoria;",
				"d) convocar, por ordem do Presidente, todas as reuniões da Diretoria, Comissão Executiva e Congresso Nacional."),
			RolePrimeiroSec: at("Específica UPH, Art. 121",
				"a) redigir e lavrar as atas das reuniões;",
				"b) substituir o Secretário Executivo em suas faltas e impedimentos eventuais."),
			RoleSegundoSec: at("Específica UPH, Art. 122",
				"a) substituir o Primeiro-Secretário em suas faltas e impedimentos eventuais;",
				"b) exercer as funções de relações públicas;",
				"c) receber os relatórios das comissões nomeadas em Congresso, e demais papéis, e conservá-los em ordem."),
			RoleTesoureiro: at("Específica UPH, Art. 123",
				"a) receber a taxa per capita;",
				"b) receber verbas e doações;",
				"c) organizar e manter em dia os livros próprios da tesouraria;",
				"d) apresentar relatórios trimestrais à Diretoria e um quadrienal ao Congresso e ao Supremo Concílio, neste caso através do Secretário Nacional;",
				"e) elaborar orçamento anual e apresentá-lo à Diretoria para aprovação;",
				"f) assinar, com o Presidente, os cheques, ordens, pagamento e balancetes da Confederação Nacional."),
		},
	},

	"SAF": {
		"local": {
			RolePresidente: {Ref: "Específica SAF, Art. 13", Itens: []string{
				"a) convocar as reuniões: da Diretoria, da Comissão Executiva e plenárias;",
				"b) elaborar planos, juntos à Diretoria, à Comissão Executiva e à Plenária;",
				"c) acompanhar as atividades da SAF, estimulando e orientando a todas na maneira de alcançar os planos aprovados;",
				"d) representar a SAF onde se fizer necessário;",
				"e) presidir as reuniões da Diretoria, da Comissão Executiva e as plenárias;",
				"f) pôr em discussão as propostas apresentadas, esclarecendo com brevidade os assuntos a serem votados;",
				"g) suspender a reunião por proposta devidamente apoiada e votada;",
				"h) receber e apresentar quaisquer visitantes ou representantes de organizações congêneres;",
				"i) apresentar relatório das atividades da SAF para aprovação da plenária, enviando cópia ao Conselho e à Federação;",
				"j) dar voto de “Minerva” no caso de empate na votação, se estiver presidindo a reunião.",
			}, Nota: "Parágrafo único — O voto de “Minerva” será dado por quem preside a reunião."},
			RoleVice: at("Específica SAF, Art. 14",
				"a) cooperar com a Presidente no exercício de suas funções;",
				"b) substituir a Presidente em suas faltas e impedimentos eventuais;",
				"c) exercer a função de relações públicas."),
			RolePrimeiroSec: {Ref: "Específica SAF, Art. 15", Itens: []string{
				"a) redigir as atas ou memória da reunião da Plenária, da Diretoria e da Comissão Executiva;",
				"b) substituir a Presidente, no impedimento da Vice-presidente.",
			}, Nota: "Parágrafo único — As atas ou memórias de reuniões podem ser feitas de forma manual ou eletrônica, em livro próprio."},
			RoleSegundoSec: at("Específica SAF, Art. 16",
				"a) substituir a Primeira Secretária em suas faltas e impedimentos;",
				"b) encarregar-se da correspondência, dando ciência dela à SAF;",
				"c) cumprimentar, em nome da SAF, as sócias em seus aniversários e em outras ocasiões especiais;",
				"d) cuidar do arquivo, da frequência e do rol das sócias."),
			RoleTesoureiro: at("Específica SAF, Art. 17",
				"a) receber verbas, anuidade individual e doações, escriturando-as devidamente em livro próprio;",
				"b) efetuar pagamentos conforme resoluções da Plenária ou da Diretoria; neste último caso, sempre ad referendum da próxima Plenária;",
				"c) efetuar o pagamento da anuidade individual à Federação;",
				"d) apresentar balancete à plenária e relatório anual ao Conselho da Igreja. Em ambos os casos, com documentação comprobatória."),
		},
		"federacao": {
			RolePresidente: {Ref: "Específica SAF, Art. 49", Itens: []string{
				"a) convocar, por meio da Secretária Executiva, e presidir as reuniões da Diretoria, da Comissão Executiva e do Congresso;",
				"b) elaborar e apresentar planos e submetê-los à aprovação da Diretoria e do(a) respectivo(a) Secretário(a) Presbiterial/Sinodal;",
				"c) apresentar relatório das atividades ao Congresso, enviando cópia deste ao Concílio competente, por meio do(a) Secretário(a) Presbiterial/Sinodal e à Confederação Sinodal/Nacional;",
				"d) representar a Federação/Sinodal onde se fizer necessário;",
				"e) assinar, com a Tesoureira, cheques, ordens de pagamento e balancetes;",
				"f) dar voto de “Minerva” no caso de empate na votação, no caso de estar presidindo a reunião.",
			}, Nota: "Parágrafo único — O voto de “Minerva” será dado por quem preside a reunião."},
			RoleVice: at("Específica SAF, Art. 50",
				"a) cooperar com a Presidente no exercício de suas funções;",
				"b) substituir a Presidente em suas faltas e impedimentos eventuais."),
			RoleSecExec: at("Específica SAF, Art. 51",
				"a) zelar pela pronta e fiel execução das resoluções emanadas do Congresso, da Comissão Executiva e Diretoria;",
				"b) receber relatórios, credenciais e os demais documentos, conservando-os em ordem e organizar o trabalho das comissões nomeadas em congresso;",
				"c) organizar e manter em dia o arquivo;",
				"d) assinar e enviar, por ordem da Presidente, toda a correspondência oficial;",
				"e) substituir a Presidente em suas faltas e impedimentos eventuais, estando ausente a Vice-presidente;",
				"f) convocar, por ordem da Presidente, todas as reuniões;",
				"g) elaborar e publicar boletins com as resoluções do Congresso, da Comissão Executiva e Diretoria;",
				"h) organizar o livro de presença nos Congressos."),
			RolePrimeiroSec: at("Específica SAF, Art. 52",
				"a) redigir e lavrar as atas ou memórias das reuniões;",
				"b) substituir a Secretária Executiva em suas faltas e impedimentos eventuais."),
			RoleSegundoSec: at("Específica SAF, Art. 53",
				"a) substituir a Primeira Secretária em suas faltas e impedimentos eventuais;",
				"b) auxiliar a Primeira Secretária nas suas funções no decorrer dos congressos;",
				"c) exercer as funções de relações públicas, enviando também correspondências não oficiais."),
			RoleTesoureiro: at("Específica SAF, Art. 54",
				"a) receber a anuidade individual;",
				"b) receber verbas e doações;",
				"c) organizar e manter em dia os livros próprios e documentos da Tesouraria;",
				"d) apresentar relatório à Diretoria, Congresso e Concílios por intermédio do(a) Secretário(a) Presbiterial/Sinodal, juntamente com o relatório da Presidente."),
		},
		"nacional": {
			RolePresidente: {Ref: "Específica SAF, Art. 75", Itens: []string{
				"a) convocar, por meio da Secretaria Executiva, e presidir as reuniões da Diretoria, da Comissão Executiva e o Congresso;",
				"b) elaborar planos e submetê-los à aprovação da Diretoria da Confederação Nacional e da Secretaria Nacional;",
				"c) apresentar relatórios das atividades da Confederação Nacional ao Congresso, e, por intermédio da Secretária Nacional, ao Supremo Concílio;",
				"d) representar a Confederação Nacional onde se fizer necessário;",
				"e) dar voto de “Minerva” nos casos de empate, na votação, caso esteja presidindo a reunião;",
				"f) assinar, com a Tesoureira, os cheques, ordens de pagamentos e balancetes da Confederação Nacional.",
			}, Nota: "Parágrafo único — O voto de “Minerva” será dado por quem preside a reunião."},
			RoleVice: at("Específica SAF, Art. 76",
				"a) cooperar com a Presidente no exercício de suas funções;",
				"b) substituir a Presidente em suas faltas e impedimentos eventuais, por ordem de idade, a começar da mais velha;",
				"c) zelar para que os objetivos, planos e realizações da Confederação Nacional sejam conhecidos e cumpridos em suas respectivas regiões."),
			RoleSecExec: at("Específica SAF, Art. 77",
				"a) assinar e enviar, por ordem da Presidente, toda a correspondência oficial da Confederação Nacional;",
				"b) receber relatórios, credenciais e os demais documentos, conservando-os em ordem e organizar o trabalho das comissões nomeadas em congresso;",
				"c) organizar e manter em dia o arquivo da Confederação Nacional;",
				"d) zelar pela pronta e fiel execução das resoluções emanadas do Congresso Nacional, Comissão Executiva e da Diretoria;",
				"e) convocar, por ordem da Presidente, todas as reuniões da Diretoria, Comissão Executiva e Congresso Nacional;",
				"f) elaborar e publicar boletins da Confederação Nacional com as resoluções das reuniões da Comissão Executiva e do Congresso Nacional;",
				"g) organizar o livro de presença nos Congressos."),
			RolePrimeiroSec: at("Específica SAF, Art. 78",
				"a) redigir e lavrar as atas das reuniões;",
				"b) substituir a Secretária Executiva em suas faltas e impedimentos eventuais."),
			RoleSegundoSec: at("Específica SAF, Art. 79",
				"a) substituir a Primeira Secretária em suas faltas e impedimentos eventuais;",
				"b) exercer as funções de relações públicas;",
				"c) organizar os protocolos de papéis que forem apresentados ao Congresso e encaminhá-los à Secretária Executiva;",
				"d) auxiliar a Primeira Secretária nas suas funções no decorrer dos congressos."),
			RoleTesoureiro: at("Específica SAF, Art. 80",
				"a) receber a anuidade individual;",
				"b) receber verbas e doações;",
				"c) organizar e manter em dia os livros próprios da tesouraria e documentos comprobatórios;",
				"d) apresentar relatórios à Diretoria e à Comissão Executiva, ao Congresso Nacional e ao Supremo Concílio, neste caso por meio da Secretária Nacional;",
				"e) elaborar o orçamento anual e apresentá-lo à Diretoria para aprovação;",
				"f) assinar, com a Presidente, cheques, ordens de pagamento e balancetes da Confederação Nacional."),
		},
	},

	"UPA": {
		AmbitoLocal: {
			RolePresidente: at("Específica UPA, Art. 15",
				"a) convocar todas as reuniões: da Diretoria e plenárias;",
				"b) elaborar planos, junto com a Diretoria, e apresentá-los à plenária;",
				"c) acompanhar as atividades da Sociedade, estimulando e orientando a todos na maneira de alcançar os planos aprovados;",
				"d) representar a Sociedade onde se fizer necessário;",
				"e) presidir as reuniões da Diretoria e as plenárias;",
				"f) pôr em discussão as propostas apresentadas;",
				"g) dar voto de desempate."),
			RoleVice: at("Específica UPA, Art. 16",
				"a) cooperar com o presidente no exercício de suas funções;",
				"b) substituir o presidente em suas faltas e impedimentos eventuais."),
			RolePrimeiroSec: at("Específica UPA, Art. 17",
				"a) providenciar o registro das reuniões da Diretoria e da plenária;",
				"b) substituir o presidente, no impedimento do vice-presidente."),
			RoleSegundoSec: at("Específica UPA, Art. 18",
				"a) encarregar-se da documentação e do registro de membros;",
				"b) substituir o primeiro secretário em suas faltas e impedimentos."),
			RoleTesoureiro: at("Específica UPA, Art. 19",
				"a) receber verbas, contribuições individuais e doações, escriturando-as devidamente em livro próprio;",
				"b) efetuar pagamentos conforme resoluções da plenária ou da Diretoria;",
				"c) efetuar o repasse percentual da contribuição individual anual diretamente para a respectiva Federação (50%), Confederação Sinodal (25%) e Confederação Nacional (25%);",
				"d) apresentar balancete financeiro à plenária e relatório anual ao Conselho da igreja;",
				"e) controlar para que todos os sócios encaminhem suas contribuições individuais mensais, quando for este o caso."),
		},
		AmbitoFederacao: {
			RolePresidente: at("Específica UPA, Art. 33",
				"a) presidir as reuniões da Diretoria e do Congresso;",
				"b) elaborar planos e submetê-los à aprovação da Diretoria da Federação e do(a) Secretário(a) Presbiterial;",
				"c) apresentar relatório das atividades da Federação, enviando cópia deste ao(à) Secretário(a) Presbiterial e à Confederação Sinodal;",
				"d) representar a Federação onde se fizer necessário;",
				"e) dar voto de desempate."),
			RoleVice: at("Específica UPA, Art. 34",
				"a) cooperar com o Presidente no exercício de suas funções;",
				"b) substituir o Presidente em suas faltas e impedimentos."),
			RoleSecExec: at("Específica UPA, Art. 35",
				"a) zelar pela pronta e fiel execução das resoluções emanadas dos Congressos e da Diretoria;",
				"b) receber os relatórios das Comissões nomeadas em Congresso e os demais papéis, conservando-os em ordem;",
				"c) organizar e manter em dia o arquivo da Federação;",
				"d) manter atualizados os dados referentes às UPAs jurisdicionadas;",
				"e) substituir o Presidente em suas faltas e impedimentos, estando ausente o Vice-Presidente;",
				"f) convocar, por ordem do Presidente, todas as reuniões da Federação."),
			RolePrimeiroSec: at("Específica UPA, Art. 36",
				"a) providenciar o registro das reuniões em livro de atas;",
				"b) substituir o Secretário Executivo em suas faltas e impedimentos eventuais."),
			RoleSegundoSec: at("Específica UPA, Art. 37",
				"a) manter o registro das UPAs federalizadas em ordem;",
				"b) auxiliar e substituir o Primeiro Secretário em suas faltas e impedimentos eventuais."),
			RoleTesoureiro: at("Específica UPA, Art. 38",
				"a) receber o percentual da contribuição individual anual correspondente das UPAs;",
				"b) receber verbas e doações;",
				"c) organizar e manter em dia os livros próprios da tesouraria;",
				"d) apresentar relatório anual ao plenário do Congresso."),
		},
		AmbitoSinodal: {
			RolePresidente: at("Específica UPA, Art. 50",
				"a) presidir as reuniões da Diretoria e do Congresso;",
				"b) elaborar planos e submetê-los à aprovação da Diretoria da Confederação Sinodal e do(a) Secretário(a) Sinodal;",
				"c) apresentar relatório das atividades da Confederação Sinodal, enviando cópia deste ao Secretário Sinodal e à Confederação Nacional;",
				"d) representar a Confederação Sinodal onde se fizer necessário;",
				"e) dar voto de desempate no caso de empate na votação de matérias e eleições."),
			RoleVice: at("Específica UPA, Art. 51",
				"a) cooperar com o Presidente no exercício de suas funções;",
				"b) substituir o Presidente em suas faltas e impedimentos."),
			RoleSecExec: at("Específica UPA, Art. 52",
				"a) zelar pela pronta e fiel execução das resoluções emanadas dos Congressos e da Diretoria;",
				"b) receber os relatórios das Comissões nomeadas em Congresso e os demais papéis, conservando-os em ordem;",
				"c) organizar e manter em dia o arquivo da Confederação Sinodal;",
				"d) manter atualizados os dados referentes às Federações jurisdicionadas;",
				"e) substituir o Presidente em suas faltas e impedimentos, estando ausente o Vice-Presidente;",
				"f) convocar, por ordem do Presidente, todas as reuniões da Confederação Sinodal."),
			RolePrimeiroSec: at("Específica UPA, Art. 53",
				"a) providenciar o registro das reuniões;",
				"b) substituir o Secretário Executivo em suas faltas e impedimentos eventuais."),
			RoleSegundoSec: at("Específica UPA, Art. 54",
				"a) manter o registro das federações em ordem;",
				"b) auxiliar e substituir o Primeiro Secretário em suas faltas e impedimentos eventuais."),
			RoleTesoureiro: at("Específica UPA, Art. 55",
				"a) receber o percentual da contribuição individual anual correspondente das UPAs;",
				"b) receber verbas e doações;",
				"c) organizar e manter em dia os livros próprios da tesouraria;",
				"d) apresentar relatório anual ao plenário do Congresso;",
				"e) acompanhar se o percentual das contribuições individuais anuais de cada UPA jurisdicionada está sendo encaminhado."),
		},
		AmbitoNacional: {
			RolePresidente: at("Específica UPA, Art. 65",
				"a) presidir as reuniões da Diretoria, da Comissão Executiva e dos Congressos;",
				"b) elaborar planos e submetê-los à aprovação da Diretoria da Confederação Nacional e do(a) Secretário(a) Nacional;",
				"c) apresentar relatórios das atividades da Confederação Nacional ao Congresso e ao Supremo Concílio;",
				"d) dar voto de desempate em casos de empate, na votação de matérias e eleições."),
			RoleVice: at("Específica UPA, Art. 66",
				"a) cooperar com o Presidente no exercício de suas funções e contribuir com o bom andamento dos trabalhos regionais;",
				"b) representar o Presidente em sua respectiva região;",
				"c) substituir o Presidente em suas faltas e impedimentos eventuais, por ordem de idade, a começar do mais velho."),
			RoleSecExec: at("Específica UPA, Art. 67",
				"a) zelar pela pronta e fiel execução das resoluções emanadas dos Congressos e da Diretoria;",
				"b) receber os relatórios das Comissões nomeadas em Congresso e os demais papéis, conservando-os em ordem;",
				"c) organizar e manter em dia o arquivo da Confederação Nacional;",
				"d) manter atualizados os dados referentes às Confederações Sinodais e Federações;",
				"e) substituir o Presidente em suas faltas e impedimentos, estando ausentes os Vice-Presidentes;",
				"f) convocar, por ordem do Presidente, todas as reuniões da Confederação Nacional."),
			RolePrimeiroSec: at("Específica UPA, Art. 68",
				"a) providenciar o registro das reuniões;",
				"b) substituir o Secretário Executivo em suas faltas e impedimentos eventuais."),
			RoleSegundoSec: at("Específica UPA, Art. 69",
				"a) substituir o Primeiro Secretário em suas faltas e impedimentos eventuais;",
				"b) receber os relatórios das comissões nomeadas em Congresso e demais papéis, e conservá-los em ordem."),
			RoleTesoureiro: at("Específica UPA, Art. 70",
				"a) receber a contribuição individual anual;",
				"b) receber verbas e doações;",
				"c) organizar e manter em dia os livros próprios da tesouraria;",
				"d) acompanhar se os percentuais de contribuições individuais anuais das UPAs estão sendo encaminhados."),
		},
	},

	"UCP": {
		"local": {
			RolePresidente: at("Específica UCP, Art. 11",
				"a) convocar todas as reuniões da diretoria e plenárias;",
				"b) elaborar planos, junto com a diretoria, e apresentá-los à plenária;",
				"c) acompanhar as atividades da Sociedade, estimulando e orientando a todos na maneira de alcançar os planos;",
				"d) representar a Sociedade onde se fizer necessário;",
				"e) presidir as reuniões da diretoria e as plenárias;",
				"f) pôr em discussão as propostas apresentadas;",
				"g) dar voto de desempate."),
			RoleVice: at("Específica UCP, Art. 12",
				"a) cooperar com o presidente no exercício de suas funções;",
				"b) substituir o presidente em suas faltas e impedimentos eventuais."),
			RolePrimeiroSec: at("Específica UCP, Art. 13",
				"a) providenciar o registro das reuniões da diretoria e da plenária;",
				"b) encarregar-se da documentação;",
				"b) exercer a função de relações públicas;", // letra duplicada no original
				"c) substituir o presidente, no impedimento do vice-presidente."),
			RoleTesoureiro: at("Específica UCP, Art. 14",
				"a) receber verbas e doações e registrá-las em livro próprio;",
				"b) efetuar pagamentos conforme resoluções da plenária ou da diretoria;",
				"c) remeter para Federação, sob a responsabilidade do(a) Conselheiro(a), a contribuição individual dos sócios, quando essa contribuição for estipulada e oficialmente comunicada pelo(a) Secretário(a) Nacional;",
				"d) apresentar relatório financeiro em cada plenária e anualmente ao Conselho da igreja."),
		},
		"federacao": {
			RolePresidente: at("Específica UCP, Art. 30",
				"a) presidir as reuniões da diretoria e do Congresso;",
				"b) elaborar planos e submetê-los à aprovação da diretoria da Federação e do Secretário Presbiterial;",
				"c) apresentar relatório das atividades da Federação, enviando cópia deste ao(à) Secretário(a) Presbiterial;",
				"d) representar a Federação onde se fizer necessário;",
				"e) dar voto de desempate."),
			RoleVice: at("Específica UCP, Art. 31",
				"a) cooperar com o Presidente no exercício de suas funções;",
				"b) substituir o Presidente em suas faltas e impedimentos."),
			RoleSecExec: at("Específica UCP, Art. 32",
				"a) zelar pela execução das decisões dos Congressos e da diretoria;",
				"b) receber os relatórios das Comissões nomeadas em Congresso e os demais papéis, conservando-os em ordem;",
				"c) organizar e manter em dia o arquivo da Federação;",
				"d) manter atualizados os dados referentes às UCPs jurisdicionadas;",
				"e) substituir o Presidente em suas faltas e impedimentos, estando ausente o Vice-Presidente;",
				"f) convocar, por ordem do Presidente, todas as reuniões da Federação."),
			RolePrimeiroSec: at("Específica UCP, Art. 33",
				"a) providenciar o registro das reuniões;",
				"b) substituir o Secretário Executivo em suas faltas e impedimentos eventuais."),
			RoleTesoureiro: at("Específica UCP, Art. 34",
				"a) receber contribuições, verbas e doações das UCPs;",
				"b) organizar e manter em dia os livros próprios da tesouraria;",
				"c) apresentar relatório anual ao plenário do Congresso;",
				"d) remeter à Confederação Sinodal, quando houver, o percentual devido das contribuições individuais dos sócios da UCP."),
		},
	},
}
