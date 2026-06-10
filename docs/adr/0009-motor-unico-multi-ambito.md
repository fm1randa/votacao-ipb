# Motor único multi-âmbito (Local, Federação, Sinodal, Nacional × 5 sociedades)

O sistema deixa de ser "eleição de Federação UMP" e passa a eleger a Diretoria de
**qualquer sociedade interna** (UMP, UPA, UPH, SAF, UCP) em **qualquer âmbito**
(Local, Federação, Confederação Sinodal, Confederação Nacional) — com **um único
motor de eleição** e uma camada de configuração/terminologia por cima, em vez de
modos ou builds separados por âmbito.

**Por que um motor serve:** o GTSI define a mecânica de eleição local (Art. 90)
e federada (Art. 91) como espelhos — voto secreto, cargo a cargo, até 3
escrutínios (3º entre os dois mais votados), maioria dos depositados, desempate
por idade. Apuração, tokens cegos, operação da mesa e telão não variam. O que
varia é **dado de configuração**: presets de cargos, regra de quórum, vocabulário
da UI (Sócio/Chamada/Plenária × Delegado/Credenciar/Congresso), existência de
natos e indicação, formato de importação do rol (ver SPEC §10.2).

**Decisões de modelagem:**
- `congress` ganha `ambito` + `sociedade`; bancos existentes migram como
  `federacao`/`UMP`. Código permanece `elector`/`congress` (a troca de termos é
  só de apresentação).
- A tabela `local` generaliza para "unidade de representação": UMP local
  (Federação), Federação (Sinodal), Sinodal (Nacional). **Só a Nacional tem dois
  níveis** — o delegado registra Sinodal **e** Federação, porque o quórum do
  Art. 49c é composto (½ das Sinodais **e** ⅓ das Federações) e decidimos
  computá-lo por inteiro em vez de deixar metade manual.
- Vices regionais da Nacional são **cargos separados** (5; 6 na SAF), sem
  validação da região do candidato (o GTSI diz só "representando").
- Âmbito/sociedade são **mutáveis apenas antes da Declaração de Abertura**
  (re-aplica o preset de cargos, com confirmação e registro no Histórico);
  depois, bloqueados — cargos com votos não podem trocar de preset.
- **Nascimento torna-se obrigatório** no rol: alimenta o desempate (90g/91g) e
  os limites de idade para concorrer no Sinodal/Nacional (Parte Comum Art. 4º
  §3–4), aplicados como exclusão da lista de votáveis.

**Alternativas descartadas:** manter o sistema UMP-Federação e bifurcar depois
(duplicaria motor e UI já maduros); âmbito sem sociedade (perderia os rótulos de
gênero da SAF e o preset de 6 vices); hierarquia de unidade em todos os âmbitos
(complexidade que só a Nacional exige).

**Consequências:** templates passam a usar um helper de termos por âmbito; o
preset de cargos vira função de (âmbito, sociedade); a importação do rol muda de
colunas conforme o âmbito; no âmbito local não existem natos, indicação nem
credencial — a aba Credenciar exibe-se como "Chamada", mantendo presença + token.
