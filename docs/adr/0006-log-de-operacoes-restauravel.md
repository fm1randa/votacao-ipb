# Log de operações restaurável (inspirado no jj)

Toda **ação da mesa** que muda o estado é registrada como uma **operação** num log
append-only (`operation`), à la Jujutsu (jj). Cada operação guarda descrição, hora e
um **snapshot em JSON** do estado da eleição **imediatamente antes** da ação. Daí:
**Desfazer** (toast após ações destrutivas) e **Restaurar para qualquer ponto** (aba
Histórico). Restaurar é, ele mesmo, uma operação registrada — então o desfazer é
reversível (há "refazer"). Nada é destruído de verdade.

**Granularidade:** operações = ações da mesa (declarar quórum, credenciar/saída,
reemitir, abrir/encerrar escrutínio, reiniciar/encerrar eleição). **Votos NÃO são
operações individuais** — acumulam dentro do estado "escrutínio aberto" e são
capturados pelo snapshot da operação seguinte. Não se desfaz "um voto" (feriria o
sigilo); desfaz-se "abrir escrutínio", que leva os votos junto.

**Por quê o snapshot é "antes":** o snapshot anterior a "Encerrar escrutínio" já
inclui os votos → desfazer o encerramento reabre a rodada **com os votos intactos**.
Isso também dá "reabrir escrutínio" de graça (= desfazer o encerrar).

**Armazenamento:** snapshot serializa as tabelas de domínio (congress, elector,
token, position, round, round_candidate, vote) em JSON, dentro do próprio banco —
**exceto** o log `operation` e o `setting` (PIN), para não se autorreferenciar nem
mexer na config ao restaurar. Restaurar usa `PRAGMA defer_foreign_keys` numa
transação (DELETE + reinsert). Log retido por inteiro (dados pequenos).

**Consequências:** cada mutação da mesa abre transação que primeiro grava a operação
(snapshot) e depois aplica a mudança. Ações destrutivas (Reiniciar/Encerrar a
eleição) exigem **modal de confirmação** e mostram **toast com Desfazer** depois.
**Restaurar também é destrutivo:** o modal mostra um **diff "agora → depois" em
linguagem de leigo** (situação, presentes, votos, status de cada cargo), carregado
sob demanda (`GET /board/restore/preview`), destacando só o que muda.
Restore via ATTACH de arquivos `.db` foi descartado em favor do JSON embutido.
