# Especificação — Votação eletrônica de Congresso (UMP, Federação Presbiterial)

Sistema offline para eleger a **Diretoria de uma Federação da UMP** durante o
Congresso anual, substituindo o escrutínio em papel. Base normativa: **GTSI –
Guia de Trabalho das Sociedades Internas da IPB (2015)**, Regimento Interno
(Art. 25–33) e **Específica UMP (Art. 26, 49–52, 90–91)**.

> Citações `(Art. N)` referem-se ao GTSI. **[decisão]** marca escolhas do projeto.

---

## 1. Contexto e escopo

- **Evento:** Congresso anual de uma Federação UMP (nível presbiterial).
- **Objetivo:** eleger a Diretoria, **cargo por cargo**, por escrutínio secreto.
- **Escopo [decisão]:** **somente a eleição da diretoria.** Votação de matérias
  (maioria dos presentes, Art. 31) fica fora — segue presencial.
- **Sem internet:** LAN local (notebook da mesa + roteador). BYOD + quiosque + telão.

## 2. Atores

| Ator | Papel |
|------|-------|
| **Delegado credenciado** | Eleitor. Vota e **pode ser votado** (qualquer delegado). |
| **Mesa Diretora** | Credencia, abre/encerra escrutínios, declara resultado. |
| **Comissão de Diplomacia** | Examina credenciais (verificação de poderes). |
| **Telão** | Exibe apuração e reconciliação. |

## 3. Regras de governança (GTSI)

### 3.1 Eleitorado e candidatos
- **Quem vota** (Art. 50, 91b): delegados efetivos credenciados — diretoria +
  secretários de atividades (natos, Art. 52) + presidentes das locais (ou
  substitutos) + ≥2 representantes por UMP local (nº no edital).
- **Quem pode ser votado [decisão, confirmada]:** **qualquer delegado
  credenciado.** Não há lista fechada pré-definida.
- **Indicação (Art. 91d) é OPCIONAL [decisão]:** a mesa *pode* registrar uma
  short-list de indicados por cargo pra focar a votação, mas no congresso pequeno
  isso quase nunca ocorre. Sem indicação → cédula lista todos os delegados.
- **Elegibilidade [decisão]:** o GTSI exige candidato membro da IPB +1 ano
  (Art. 91c) e sócio ativo +1 ano (Art. 25 PU), **mas o software NÃO verifica
  isso** — responsabilidade 100% humana (Comissão de Diplomacia).
- **Eleito não concorre a outro cargo:** quem já foi eleito num cargo decidido sai
  da cédula dos cargos seguintes e o servidor recusa votos nele (não se acumulam
  cargos na diretoria). Desfazer a eleição de um cargo devolve a pessoa à cédula.

### 3.2 Credenciamento = Verificação de Poderes (Art. 14, programa Art. 53)
- Primeira sessão: composição da mesa, devocional, **recebimento de credenciais**,
  entrada de documentos, **chamada e verificação de quórum**, declaração de abertura.
- Delegados assinam o **Livro de Presença**; credenciais atrasadas examinadas
  conforme chegam. Diretoria/secretários são natos (sem credencial — Art. 52).
- No sistema: cada credenciado recebe um **token cego**. Tokens entregues =
  presentes.

### 3.3 Quórum (Art. 26, 49)
- **Eleição sempre exige mais da metade** — não há quórum reduzido de 2ª
  convocação para eleições (o de 1/3 vale só para reuniões ordinárias).
- **Art. 49a:** o quórum do Congresso da Federação é **"mais da metade das UMPs
  locais"** — medido por **representação de sociedades locais**, não por cabeça.
- **No sistema [decisão]:** painel da mesa **mostra os dois** — (i) UMPs locais
  representadas (≥1 delegado presente) sobre total de locais, e (ii) headcount de
  delegados presentes. A **mesa declara** o quórum oficial pelas locais. Sem
  quórum declarado, não se abre escrutínio (gate).

### 3.4 Eleição da Diretoria (Art. 91 — regra UMP específica)
- **(a)** Escrutínio **secreto** por cédula durante o Congresso (Art. 29c torna o
  sigilo obrigatório → justifica o token cego).
- **(d)** **Cargo por cargo**, podendo haver indicação opcional de nomes.
- **(e)** Se no **1º e 2º escrutínio** ninguém alcançar a maioria, faz-se um
  **3º escrutínio com os dois mais votados** (segundo turno).
- **(f)** Eleito quem obtiver **mais da metade dos votos**.
- **(g)** **Empate no 3º escrutínio → desempate por MAIOR IDADE do candidato**
  (não é voto de Minerva). Automatizável só se houver data de nascimento; senão a
  mesa registra o vencedor.
- **(h)** Posse pelo Secretário de Causas logo após a eleição (fora do software).

### 3.5 Cargos da Diretoria da Federação UMP (Art. 26a) — ordem de eleição
1. Presidente → 2. Vice-presidente → 3. Secretário Executivo →
4. 1º Secretário → 5. 2º Secretário → 6. Tesoureiro

**Cargos configuráveis [decisão 2026-06-10]:** federações menores podem desabilitar
**Vice-presidente, Secretário Executivo e 2º Secretário** (no wizard e em Ajustes);
Presidente, 1º Secretário e Tesoureiro são obrigatórios. Sem o 2º Secretário, o 1º
exibe-se apenas como **"Secretário"**. Cargo **em eleição ou decidido não pode ser
desabilitado** (desfaça pelo Histórico antes).

> ⚠️ **Base normativa (verificada 2026-06-10):** para Federações, o Art. 26a é
> **prescritivo** — diretoria completa, **sem** cláusula de mínimo. A diretoria
> reduzida "Presidente, Secretário e Tesoureiro" só tem previsão **expressa** para
> a **sociedade local** (Art. 13, "em casos especiais"; idem o "quando houver" do
> §3º). Reduzir a diretoria da Federação é prática por **analogia** — confirme com
> o Secretário Presbiterial antes do congresso (ver §9).

## 4. Regra de cálculo da maioria [decisão]

- Base do limiar: **todos os votos depositados** (candidatos + brancos + nulos).
- **Limiar = ⌊total_depositado / 2⌋ + 1.** Eleito = `votos ≥ limiar`.
- **Sequência por cargo:**
  - Escrutínio 1 → eleito se alguém ≥ limiar; senão escrutínio 2.
  - Escrutínio 2 → idem; senão **escrutínio 3 restrito aos 2 mais votados**.
  - Escrutínio 3 → eleito quem ≥ limiar. **Se brancos/nulos impedirem a metade+1,
    vence o MAIS VOTADO** (plurality) **[decisão]**; **empate exato → maior idade**
    (Art. 91g).
- **Empate na definição do top-2** (ex.: empate no 2º lugar entrando no 3º turno):
  a mesa decide quem entra, registrado em ata.

## 5. Modelo de dados (proposto, revisado)

```
congress (federação, ano)
  └─ local (UMP local: nome)                 ← base do quórum por representação
  └─ position (cargo: nome, seq 1..6, status: pendente|em_eleicao|decidido,
               eleito_elector_id?)
       ├─ nomination (elector_id)            ← indicação OPCIONAL (short-list)
       └─ round (nº 1..3; status; restrito_top2: lista de elector_id ou vazio)
            └─ vote (round_id, token, kind, votee_elector_id)
elector (delegado: nome, local_id?, nato, [nascimento?], presente)
token   (pilha cega: token, ativo)            ← NÃO mede presença (ADR-0002)
```

Notas do modelo:
- **`vote` referencia o delegado VOTADO** (`votee_elector_id`, identidade
  pública), **nunca o votante**. O sigilo segue protegido pelo token cego.
- Queima atômica permanece: **`UNIQUE(round_id, token)`**.
- **Presença é da PESSOA, não do token** (ADR-0002): `elector.presente` é o
  Livro de Presença, **reversível** (a Mesa registra saída/reentrada). Quórum e
  reconciliação contam `elector.presente`, nunca tokens. Reemitir token (perda) só
  cria um token a mais em circulação, sem mexer na presença.
- **Nato** (`elector.nato`): vota, conta no headcount, mas `local_id` nulo → não
  soma ao quórum por UMPs locais.
- **Votável = delegados presentes**, exibidos como "Nome — UMP Local" (desfaz
  homônimos). Sem normalização de nomes: escolhe-se da lista, não se digita.
- Cédula de um escrutínio lista, nesta ordem de precedência: o **top-2** (se 3º
  turno restrito) → senão as **indicações** da position (se houver) → senão
  **todos os delegados credenciados**.
- `local` + `elector.local_id` dão o quórum por representação; headcount sai de
  `elector.presente` / tokens entregues.
- `nascimento` no elector é **opcional**; sem ele, desempate por idade é manual.

## 6. Fluxos

1. **Credenciamento:** cadastra/importa rol de delegados e suas UMPs locais; ao
   credenciar, entrega token cego. Quórum (locais + headcount) atualiza ao vivo.
2. **Declarar quórum:** mesa confirma quórum (gate p/ abrir escrutínios).
3. **Abrir cargo:** mesa seleciona o cargo da vez (na ordem); opcionalmente
   registra indicações; abre o escrutínio 1.
4. **Votar:** delegado digita o token e escolhe um delegado / branco / nulo.
   Token queima; voto duplo barrado.
5. **Apurar:** telão mostra contagem, limiar e reconciliação ao vivo.
6. **Encerrar escrutínio:** eleito → cargo `decidido`. Senão abre o próximo
   (3º = só top-2; plurality + idade se brancos travarem).
7. **Próximo cargo:** repete até os 6 decididos.

## 6A. Operação e UX (decisões do grilling, 2026-06-09)

- **Encerramento do escrutínio:** manual pela Mesa, com **indicador de apoio** no
  painel (`votaram X de Y presentes`); sugere encerrar quando todos votaram, mas
  quem decide é a Mesa (ato formal; evita travar se alguém opta por não votar).
- **Telão com escrutínio aberto:** mostra **só progresso** (quantos votaram),
  **nunca placar por candidato** — placar só após encerrar (ADR-0001).
- **Branco e Nulo** coexistem e são distintos: Branco = abstenção, Nulo =
  rejeição/protesto. Ambos entram no denominador (§4); reportados à parte na ata.
- **Acesso da Mesa (`/board`):** protegido por **PIN da Mesa** (1x por dispositivo,
  fica na sessão). Área do eleitor segue aberta — o token é que protege o voto.
- **Fluxo do delegado (redesenho do usuário, Excalidraw 2026-06-09):** home é um
  seletor de papel (Sou delegado / Sou da mesa / Telão). O token é digitado **uma
  vez**, num login ("Área do delegado"); vira sessão (cookie HttpOnly, 12h). A área
  do delegado é um hub **vivo** com 3 estados — votação **aberta** (Votar), **fechada**
  ("Nenhuma votação aberta") e **já votou** ("Você já votou! Obrigado.") — e **Sair**.
  A tela de voto não pede mais código (só a escolha + confirmação). O quiosque é
  coberto pelo Sair após votar (não há mais modo `?kiosk`).
- **Presença reversível:** a Mesa registra saída/reentrada de delegado; quórum e
  reconciliação refletem quem está de fato presente.
- **Relatório imprimível:** o sistema gera uma view imprimível com os dados da
  Verificação de Poderes (credenciados, presentes, UMPs locais representadas) e o
  resultado + reconciliação de cada escrutínio, para anexar à ata e subsidiar o
  juízo de legalidade (Art. 91h).

## 6B. UI/UX (grilling de design, 2026-06-09)

Direção guiada por frontend-design (distinção, anti-genérico) + emil-design-eng
(correção invisível, movimento proposital), filtrada pelo contexto (leigos/idosos,
offline, confiança > impacto).

- **Estética: identidade visual da IPB** ([ADR-0004](docs/adr/0004-identidade-visual-ipb.md)) —
  verde presbiteriano (`#0d5131` + verde médio), logo oficial (sarça ardente), sobre
  branco-quente. (Substitui a rodada cívica vinho/creme.)
- **Tema por superfície:** claro (eleitor/mesa, legibilidade) + escuro verde-tingido
  (telão, projetor) com o logo na variante branca.
- **Tipografia: Roboto embutida** (`//go:embed`, offline), corpo + títulos, por
  fidelidade ao site da IPB (trade-off de acessibilidade aceito — ADR-0004).
- **Logo** discreto nas telas de ação; maior na home e no telão.
- **Tempo real** ([ADR-0007](docs/adr/0007-tempo-real-htmx-sse.md)): telão, eleitor e mesa
  atualizam ao vivo via **SSE + htmx** embutidos (offline). Toda mutação faz `Broadcast`;
  o SSE manda um `tick` e o htmx re-busca o fragmento da tela. Degrada sem JS; telão tem
  poll de reserva. ADR-0001 preservado (fragmento do telão só mostra progresso enquanto aberto).
- **Ações sem reload** ([ADR-0008](docs/adr/0008-acoes-sem-reload-htmx.md)): ações da mesa
  são `hx-post` → `204` + toast via `HX-Trigger`; o SSE atualiza as regiões vivas. Credenciar/
  reemitir mostram o código num **modal** e atualizam a linha/contadores via **OOB** (busca
  preservada). Sem JS, degrada para POST→redirect. Acabou o `?toast=`.
- **Reset/ciclo de vida** ([ADR-0006](docs/adr/0006-log-de-operacoes-restauravel.md)):
  log de operações restaurável (à la jj). Aba **Histórico** = op log com Desfazer/Restaurar
  a qualquer ponto; **Reiniciar** e **Encerrar** a eleição na zona de perigo. Toda ação
  destrutiva pede **modal de confirmação** e mostra **toast com Desfazer**. Tudo reversível.
- **Navegação** ([ADR-0005](docs/adr/0005-casca-de-navegacao-da-mesa.md)): a mesa é uma
  casca com nav persistente (sidebar no desktop / barra inferior no mobile) e 4 abas —
  Escrutínio · Credenciamento · Telão · Relatório. Escrutínio = cargo da vez + uma ação
  (ordem fixa) + progresso; Credenciamento = página dedicada com busca (nome/igreja) e
  filtros. Eleitor permanece linear, com "Início" no topo.
- **Confirmação antes do voto:** tela de revisão ("votando em Fulano — confirmar?")
  porque o voto queima o token e é irreversível.
- **Movimento contido e proposital:** feedback de toque (`scale .97`), entradas sutis,
  barra do telão suave, **um** momento de deleite na revelação do ELEITO;
  respeita `prefers-reduced-motion`; durações < 300ms, curvas ease-out custom.
- **Token: 4 caracteres** (risco de brute-force aceito — [ADR-0003](docs/adr/0003-token-de-4-caracteres.md)),
  digitado num campo único no login do delegado (§6A).

## 6C. Onboarding e configuração (grilling 2026-06-09)

- **Primeira configuração: wizard de 3 passos** — (1) PIN da Mesa, (2) Congresso
  (federação + ano; os **6 cargos do GTSI** e a pilha de tokens são criados
  automaticamente), (3) Delegados (com "concluir depois").
- **Cadastro do rol:** form individual (nome, igreja, nascimento opcional, nato) **e**
  colar lista (uma linha = `Nome; Igreja`; igrejas inexistentes são criadas).
- **Depois do setup:** delegados são geridos **dentro da aba Credenciar** (adicionar,
  colar lista, editar; remover só quem nunca foi credenciado); federação/ano numa tela
  **Ajustes** (engrenagem no topo da aba Escrutínio). Tudo passa pelo log de operações.
- **Sem congresso configurado**, as telas públicas convidam a configurar (sem erro).
- `-seed` permanece como ferramenta de desenvolvimento.

## 7. Sigilo e auditoria
- **Sigilo:** `vote` nunca referencia o votante; elo único é o token cego, cuja
  entrega não registra identidade. Candidato/votado é público; voto é secreto.
- **Reconciliação por escrutínio:** `depositados ≤ presentes`; abstenções =
  presentes − depositados. No telão.
- **Trilha:** equivalente digital da Ata de Verificação de Poderes (total de
  credenciados/presentes, locais representadas) e do resultado de cada escrutínio.
- **Recuperação:** WAL persiste cada voto; reinício sem perder estado.

## 8. Fora de escopo
- Votação de matérias/propostas/relatórios (Art. 31).
- Posse e atos litúrgicos (Art. 91h).
- Outros níveis (Sinodal/Nacional) e outras sociedades.

## 9. Itens em aberto (não-software)
1. **Aval da mesa ao voto eletrônico** (formalidade, não bloqueio). Há
   **precedente institucional**: a IPB já adota voto digital — o sistema **SEO/CSM**
   (desenvolvido em 2020, doado à IPB) elege oficiais locais (pastores, presbíteros,
   diáconos) de forma online. Porém o SEO/CSM **não cobre diretorias conciliares nem
   sociedades internas**, e é online — exatamente o nicho que este projeto preenche
   (diretoria de Federação UMP, **offline**). Resta apenas o **registro do aval** do
   Secretário Presbiterial / mesa para o congresso, blindando a "legalidade da
   eleição" (Art. 91h). O GTSI exige cédula secreta (Art. 29c) e o token cego
   preserva o sigilo; a reconciliação do telão dá números auditáveis à mesa.
   Ref.: https://www.executivaipb.com.br/seo/
2. **Parâmetros do edital:** nº de representantes por UMP local (Art. 50a) e a
   lista de UMPs locais federadas (base do quórum) — dados a carregar no rol.
3. **Coletar data de nascimento** dos delegados para automatizar o desempate por
   idade (Art. 91g), ou deixar o desempate manual na mesa?
4. **Diretoria reduzida na Federação:** o Art. 26a prevê a diretoria completa, sem
   cláusula de mínimo (a redução expressa é só da sociedade local, Art. 13). Se a
   federação for usar menos cargos (§3.5), validar com o Secretário Presbiterial.
