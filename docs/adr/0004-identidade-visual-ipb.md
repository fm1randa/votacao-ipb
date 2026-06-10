# Identidade visual alinhada à IPB

A UI adota a identidade da Igreja Presbiteriana do Brasil (ipb.org.br):
**paleta verde** (verde profundo `#0d5131` + verde médio), **logo oficial** (sarça
ardente; variante colorida no claro, branca no telão escuro) e **fonte Roboto**.
Substitui a paleta vinho/creme e as fontes Fraunces + Atkinson Hyperlegible da
rodada anterior.

**Por quê:** o app é de uma Federação da IPB; coesão com a marca da denominação
vale mais, aqui, que uma identidade própria distinta.

**Trade-offs aceitos:**
- **Roboto** é genérica (o oposto do que o frontend-design recomenda) e abrimos mão
  da **Atkinson Hyperlegible**, escolhida pela legibilidade para idosos. Mitiga-se
  com corpo grande + alto contraste. Decisão do usuário, por fidelidade à IPB.
- **Logo oficial** é marca registrada — o **aval de uso é da federação**. Os arquivos
  (`logo_ipb.png`, `logo_ipb3.png`) vieram do site oficial e estão embutidos offline.

Fontes e logo são servidos de `/assets` via `//go:embed` (sem internet no evento).
