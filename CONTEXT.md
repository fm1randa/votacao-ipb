# Votação de Congresso (UMP — Federação Presbiterial)

Contexto da eleição da Diretoria de uma Federação da UMP durante o Congresso
anual, por escrutínio secreto e offline. A linguagem segue o GTSI (Guia de
Trabalho das Sociedades Internas da IPB).

## Language

**Congresso**:
A assembleia anual da Federação onde se elege a Diretoria.
_Avoid_: Assembleia, sessão (são partes do Congresso, não o todo).

**Federação**:
A união das UMPs locais de um Presbitério; o corpo cuja Diretoria se elege.

**UMP Local**:
A sociedade de mocidade de uma igreja local. Base da representação para quórum.
_Avoid_: Sociedade (ambíguo — pode ser local ou interna em geral).

**Delegado**:
Pessoa com direito a voto no Congresso, uma vez credenciada.
_Avoid_: Eleitor, sócio, votante, membro (todos mais amplos que Delegado).

**Mesa**:
O grupo que dirige e opera o Congresso (e o sistema): credencia, abre/encerra
escrutínios e declara resultados.
_Avoid_: Administração, diretoria (a Diretoria é o que se elege, não quem opera).

**Credenciamento**:
O ato de marcar um Delegado como presente (Presença) e entregar-lhe um Token. No
GTSI: "Verificação de Poderes".
_Avoid_: Check-in, login.

**Presença**:
O registro público de que um Delegado compareceu (equivale ao Livro de Presença).
Base do Quórum e da Reconciliação. Conta pessoas, não Tokens.

**Token**:
A credencial cega de votação entregue no Credenciamento. Identifica um Voto sem
identificar o Delegado que o emitiu. Um por Delegado, válido todo o Congresso.
**Não** serve para medir Presença.
_Avoid_: Senha, cédula (a cédula é o ato de votar; o Token é a chave anônima).

**Cargo**:
Um posto eletivo da Diretoria (Presidente, Vice, etc.). Eleito isoladamente.
Federações menores podem desativar Vice-presidente, Secretário Executivo e
2º Secretário; sem o 2º, o 1º Secretário exibe-se apenas como "Secretário".
_Avoid_: Eleição (a Eleição é do Congresso inteiro; um posto isolado é um Cargo).

**Escrutínio**:
Uma rodada de votação secreta para um único Cargo. Há até 3 por Cargo.
_Avoid_: Votação, turno (turno serve só para o 3º Escrutínio, o "segundo turno").

**Voto**:
A escolha secreta registrada num Escrutínio: um Delegado votado, ou Branco/Nulo.

**Branco**:
Voto de abstenção — o Delegado vota sem escolher ninguém. Conta no denominador.

**Nulo**:
Voto de rejeição/protesto — "nenhum dos nomes". Conta no denominador, reportado à
parte do Branco.

**Nato**:
Delegado que é membro por ofício (Diretoria atual e Secretários de Atividades) —
vota, mas não apresenta credencial e não representa uma UMP Local.
_Avoid_: Ex-officio (use o termo do GTSI, "nato").

**Quórum**:
A presença mínima para o Congresso funcionar: mais da metade das UMPs Locais.
Conta Locais representadas, não pessoas — Natos não somam ao denominador.

**Indicação**:
Nome proposto pelo Plenário para um Cargo (opcional). Sem Indicação, qualquer
Delegado credenciado pode receber Votos.
_Avoid_: Candidatura, chapa (não há chapas; a unidade é o Cargo).

**Operação**:
Uma ação da Mesa que muda o estado (credenciar, abrir/encerrar Escrutínio,
reiniciar/encerrar a Eleição). Registrada no Histórico com um retrato do estado
anterior. Votos não são Operações.

**Histórico**:
O log append-only de Operações. Permite **Desfazer** a última e **Restaurar** para
qualquer ponto anterior. Inspirado no log de operações do jj.

**Reiniciar a eleição**:
Operação que apaga Escrutínios e Votos e zera Presença/credenciamento, mantendo o
rol, os Cargos e os Tokens. Volta ao "pronto pra começar". Restaurável.

**Apuração**:
A contagem dos Votos de um Escrutínio e a aplicação da regra de maioria.

**Reconciliação**:
A conferência de que os Votos depositados não excedem os presentes, com o detalhe
de brancos, nulos e abstenções.

## Relationships

- Um **Congresso** elege vários **Cargos**, um de cada vez (cargo por cargo).
- Um **Cargo** é decidido por um a três **Escrutínios** (o 3º entre os dois mais votados).
- Um **Delegado** recebe um **Token** no **Credenciamento** e emite um **Voto** por **Escrutínio**.
- Um **Voto** referencia o **Delegado votado** (público), nunca o votante (sigilo).
- O **Quórum** mede-se por **UMPs Locais** representadas.

## Example dialogue

> **Dev:** "Quando abre o **Escrutínio** do Presidente, quem pode receber **Voto**?"
> **Mesa:** "Qualquer **Delegado** credenciado — salvo se o Plenário fez **Indicação**,
> aí só os indicados. No 3º **Escrutínio**, só os dois mais votados."
> **Dev:** "E o **Token** do **Delegado** vale só nesse **Escrutínio**?"
> **Mesa:** "O **Token** é um só pro **Congresso** todo; ele queima a cada **Escrutínio**
> em que vota, mas serve para todos os **Cargos**."

## Flagged ambiguities

- "Eleição" foi usada para um Cargo isolado — resolvido: um posto é um **Cargo**; a
  Eleição é do Congresso inteiro.
- "Eleitor"/"sócio" usados para quem vota — resolvido: no Congresso, quem vota é o
  **Delegado** credenciado.
