# Releases contínuas da main: versão por contagem de commits e chave de assinatura única

> **Status: ACEITA (2026-06-11) — decisões cravadas pelo mantenedor em sessão de
> grill; implementação planejada (issue #10).**

## Contexto — distribuição manual e sem identidade

Todo artefato hoje nasce na máquina do mantenedor: o `.exe` da federação e o
binário do Termux são cross-compiles à mão (README), e o APK é `assembleDebug`
assinado com a chave debug local, com `versionCode = 1` e
`versionName = "0.1-spike"` congelados no Gradle. O binário Go não sabe a
própria versão — no dia da eleição, sem internet, não há como olhar para o
aparelho da Mesa e afirmar qual código está rodando (por exemplo, confirmar que
a correção do sigilo do voto, ADR-0013, está presente antes de abrir o
escrutínio). E com `versionCode` fixo, **nenhuma atualização instala por cima
da anterior**.

O mantenedor quer o inverso: todo estado publicado da `main` instalável, sem
intervenção humana.

## Decisão

1. **Gatilho: push na `main`.** Cada push gera uma release construída na ponta
   (um push com N commits = 1 release — o GitHub Actions dispara por push, não
   por commit; rajadas se agrupam sozinhas). A release só sai se
   `build + vet + test` passarem: um único workflow `release.yml` com job de
   teste seguido do job de release (`needs`), porque o mantenedor também faz
   commits diretos na `main` — não dá para assumir que "já passou no CI do PR".
   O `ci.yml` existente passa a rodar **só em `pull_request`** (hoje ele roda em
   push de qualquer branch *e* em PR, duplicando execuções).

2. **Versão = contagem de commits da `main`.** `N = git rev-list --count HEAD`;
   tag `v0.N`; no Android, `versionCode = N` e `versionName = "0.N"`; no Go, a
   versão entra por `-ldflags -X` e aparece no rodapé da casca da Mesa
   (verificável offline, no aparelho, em segundos). O número é monotônico,
   derivável localmente sem CI, e uma colisão de tag (história reescrita) faz o
   workflow falhar alto em vez de publicar silenciosamente errado.

3. **Cinco artefatos por release:** `windows/amd64` (`votacao.exe`, o notebook
   da federação), `linux/amd64`, `linux/arm64` para Termux **já corrigido**
   (PIE + PT_TLS, dispensando o `termux-elf-cleaner` no aparelho — a receita do
   README encurta um passo), `darwin/arm64`, e o APK assinado.

4. **Uma chave de assinatura, para sempre.** Keystore de release dedicado,
   gerado uma única vez, vivendo em GitHub Secrets, com cópia de resgate no
   gerenciador de senhas do mantenedor. O Android só instala atualização sobre
   assinatura idêntica — a chave acompanha o app pela vida toda.

5. **Repo público.** Releases baixáveis sem conta GitHub (mesário/outra igreja
   no dia), minutos de Actions ilimitados. A história foi auditada antes da
   virada (2026-06-11): nenhum `.db`, chave ou credencial jamais commitado;
   fixtures de teste só com nomes fictícios.

## Alternativas rejeitadas

- **SemVer por conventional commits** (release-please/semantic-release): os
  commits da casa são português livre ("Ler atribuições: texto do GTSI…");
  exigiria mudar o hábito de commit para sempre, por uma semântica
  major.minor.patch que um sistema interno de apuração não consome.
- **Tag manual**: contraria a automação pedida — reintroduz o humano que decide
  o número.
- **`run_number` do Actions**: não significa nada sobre o repo, não é derivável
  fora do CI e zera se o workflow for renomeado.
- **CalVer**: legível, mas o `versionCode` precisaria de uma segunda fonte de
  monotonicidade — duas numerações convivendo.
- **debug.keystore commitado**: manteria o app debuggable (e o `adb run-as`
  como rota de resgate das urnas), mas põe a chave dentro de um repositório
  prestes a ficar público e publica builds de debug como produto.
- **Encadear `ci.yml` → release via `workflow_run`**: dois workflows acoplados
  por gatilho indireto; mais difícil de ler e depurar quando falhar.
- **Release sem gate de teste**: para um sistema de apuração, um APK quebrado
  instalado na véspera é exatamente o acidente que o gate evita.

## Consequências

- **A `main` nunca pode sofrer force-push.** A contagem de commits regrediria e
  o `versionCode` deixaria de ser monotônico; a colisão de tag derruba o
  workflow de propósito.
- **Perder o keystore órfã todos os aparelhos.** Sem a chave, nenhuma
  atualização in-place nunca mais — só desinstalar, e desinstalar **apaga as
  eleições armazenadas** (não existe hoje rota de exportação/backup das urnas
  no app; em build de release nem `adb run-as` funciona). Guardar a cópia de
  resgate não é opcional.
- **Migração única e inevitável:** o app debug instalado hoje não aceita o
  primeiro APK assinado pelo CI (assinatura diferente). Uma desinstalação, uma
  vez, antes da primeira release.
- **As issues do repositório ficam públicas na virada** — e a issue #8 (índice
  da auditoria) documenta postura de segurança, inclusive a decisão de não ter
  rate-limit no PIN da Mesa. Revisar as issues antes de tornar o repo público.
- A receita Termux do README perde o passo do `termux-elf-cleaner` quando
  apontar para o artefato da release.
