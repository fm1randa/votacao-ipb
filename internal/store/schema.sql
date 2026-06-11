-- Schema do sistema de votação (Diretorias de Sociedades Internas da IPB).
-- Base normativa: GTSI 2015 (Art. 90 local; Art. 26, 49–52, 91 federados). Ver SPEC.md §10.
--
-- Princípios centrais:
--  * Sigilo: `vote` referencia o delegado VOTADO (público), nunca o votante. O
--    voto guarda vote_key = HMAC(salt do round, token), não o token; a salt é
--    anulada no encerramento, severando o elo voto↔token de vez (ADR-0013).
--  * Presença é da PESSOA, não do token (ADR-0002): quórum/reconciliação contam
--    `elector.presente` (reversível), nunca tokens entregues.
--  * Queima atômica do token: UNIQUE(round_id, vote_key) em `vote`.
--  * Multi-âmbito (ADR-0009): um motor só; âmbito+sociedade são configuração.

PRAGMA foreign_keys = ON;

-- Configurações simples chave/valor (ex.: hash do PIN da Mesa, definido na 1ª vez).
CREATE TABLE IF NOT EXISTS setting (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

-- O evento (Plenária local ou Congresso federado). Em geral há um só por banco.
-- `abertura_declarada` é o gate computado da Declaração de Abertura (ADR-0010):
-- só liga com quórum atingido; sem ela não se abre escrutínio.
CREATE TABLE IF NOT EXISTS congress (
    id                  INTEGER PRIMARY KEY,
    ambito              TEXT    NOT NULL DEFAULT 'federacao'
                        CHECK (ambito IN ('local','federacao','sinodal','nacional')),
    sociedade           TEXT    NOT NULL DEFAULT 'UMP'
                        CHECK (sociedade IN ('UMP','UPA','UPH','SAF','UCP')),
    nome                TEXT    NOT NULL,  -- nome da entidade (ex.: "Federação de UMP do PRNT")
    ano                 INTEGER NOT NULL,
    abertura_declarada  INTEGER NOT NULL DEFAULT 0,
    encerrada           INTEGER NOT NULL DEFAULT 0,  -- eleição encerrada (só leitura)
    criado_em           TEXT    NOT NULL DEFAULT (datetime('now'))
);

-- Log de operações da Mesa (ADR-0006), à la jj: append-only, cada linha guarda um
-- retrato JSON do estado de DOMÍNIO imediatamente ANTES da operação. Base do
-- Desfazer/Restaurar. NÃO é incluída nos snapshots (evita auto-referência).
CREATE TABLE IF NOT EXISTS operation (
    id         INTEGER PRIMARY KEY,
    criado_em  TEXT    NOT NULL DEFAULT (datetime('now')),
    descricao  TEXT    NOT NULL,
    snapshot   TEXT    NOT NULL
);

-- Unidade de representação, base do quórum federado (Art. 49): UMP local na
-- Federação, Federação na Sinodal, Sinodal na Nacional. `nivel` 0 = unidade
-- primária; 1 = subunidade (só no âmbito nacional: a Federação do delegado,
-- p/ o critério de ⅓ do Art. 49c). Sem uso no âmbito local.
CREATE TABLE IF NOT EXISTS local (
    id           INTEGER PRIMARY KEY,
    congress_id  INTEGER NOT NULL REFERENCES congress(id) ON DELETE CASCADE,
    nome         TEXT    NOT NULL,
    nivel        INTEGER NOT NULL DEFAULT 0
);

-- Votante do rol: Sócio Ativo (âmbito local) ou Delegado (federados).
-- `nato` => sem credencial e sem unidade (Art. 52; só federados).
-- `credenciado` é monótono (já recebeu token alguma vez); `presente` é reversível
-- (a Mesa registra saída/reentrada) e é o que conta para quórum/reconciliação.
-- `nascimento` é obrigatório no app (desempate Art. 90g/91g + limites de idade).
CREATE TABLE IF NOT EXISTS elector (
    id            INTEGER PRIMARY KEY,
    congress_id   INTEGER NOT NULL REFERENCES congress(id) ON DELETE CASCADE,
    nome          TEXT    NOT NULL,
    local_id      INTEGER REFERENCES local(id),   -- nulo para nato e no âmbito local
    sub_local_id  INTEGER REFERENCES local(id),   -- Federação do delegado (só nacional)
    nato          INTEGER NOT NULL DEFAULT 0,
    nascimento    TEXT,
    credenciado   INTEGER NOT NULL DEFAULT 0,
    presente      INTEGER NOT NULL DEFAULT 0,
    criado_em     TEXT    NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_elector_congress ON elector(congress_id);

-- Pilha de tokens cegos. `entregue` é só controle de pilha (NÃO mede presença).
-- Tokens entregues podem exceder os presentes pelo nº de reemissões (perda).
CREATE TABLE IF NOT EXISTS token (
    token        TEXT    PRIMARY KEY,
    congress_id  INTEGER NOT NULL REFERENCES congress(id) ON DELETE CASCADE,
    entregue     INTEGER NOT NULL DEFAULT 0,
    entregue_em  TEXT
);

-- Cargo: posto eletivo da Diretoria, eleito em sequência. O conjunto vem do
-- preset (âmbito × sociedade — SPEC §10.2); `role` identifica o papel
-- independentemente do nome exibido (gênero, regiões da Nacional).
-- `ativo`: cargos opcionais do âmbito podem ser desabilitados (SPEC §3.5).
CREATE TABLE IF NOT EXISTS position (
    id                 INTEGER PRIMARY KEY,
    congress_id        INTEGER NOT NULL REFERENCES congress(id) ON DELETE CASCADE,
    nome               TEXT    NOT NULL,
    role               TEXT    NOT NULL DEFAULT '',
    seq                INTEGER NOT NULL,
    ativo              INTEGER NOT NULL DEFAULT 1,
    status             TEXT    NOT NULL DEFAULT 'pendente'
                       CHECK (status IN ('pendente', 'em_eleicao', 'decidido')),
    eleito_elector_id  INTEGER REFERENCES elector(id),
    UNIQUE (congress_id, seq)
);

-- Escrutínio: rodada 1..3 de um cargo. `runoff`=1 no 3º (entre os 2 mais votados).
-- `vote_key_salt` (spike 004 / ADR-0013): segredo aleatório do escrutínio (32 bytes)
-- usado para derivar vote.vote_key = HMAC(salt, token). É ANULADO no encerramento,
-- severando para sempre o elo voto↔token (sigilo do voto). NULL = escrutínio fechado.
CREATE TABLE IF NOT EXISTS round (
    id            INTEGER PRIMARY KEY,
    position_id   INTEGER NOT NULL REFERENCES position(id) ON DELETE CASCADE,
    numero        INTEGER NOT NULL,             -- 1, 2 ou 3
    status        TEXT    NOT NULL DEFAULT 'aberto'
                  CHECK (status IN ('aberto', 'encerrado')),
    runoff        INTEGER NOT NULL DEFAULT 0,
    aberto_em     TEXT    NOT NULL DEFAULT (datetime('now')),
    encerrado_em  TEXT,
    vote_key_salt BLOB,
    UNIQUE (position_id, numero)
);

-- Conjunto votável restrito de um escrutínio:
--  * vazio  => votável = todos os delegados presentes (caso comum);
--  * com linhas => indicação opcional (Art. 91d) OU os 2 mais votados (runoff).
CREATE TABLE IF NOT EXISTS round_candidate (
    round_id     INTEGER NOT NULL REFERENCES round(id) ON DELETE CASCADE,
    elector_id   INTEGER NOT NULL REFERENCES elector(id),
    PRIMARY KEY (round_id, elector_id)
);

-- Voto. SEM votante. `votee_elector_id` = delegado votado (identidade pública).
-- `vote_key` (spike 004 / ADR-0013) NÃO guarda o token: guarda HMAC(round.salt, token),
-- valor opaco enquanto a salt vive e IRRECUPERÁVEL depois que ela é anulada no
-- encerramento — é o que torna o voto secreto mesmo com o .db inteiro em mãos.
-- A UNIQUE(round_id, vote_key) é a queima (impede voto duplo no escrutínio): como
-- o HMAC é determinístico enquanto a salt existe, o mesmo token rende o mesmo
-- vote_key e colide.
CREATE TABLE IF NOT EXISTS vote (
    id                INTEGER PRIMARY KEY,
    round_id          INTEGER NOT NULL REFERENCES round(id) ON DELETE CASCADE,
    vote_key          TEXT    NOT NULL,
    kind              TEXT    NOT NULL CHECK (kind IN ('candidato', 'branco', 'nulo')),
    votee_elector_id  INTEGER REFERENCES elector(id),
    criado_em         TEXT    NOT NULL DEFAULT (datetime('now')),
    UNIQUE (round_id, vote_key),
    -- 'candidato' exige votee; 'branco'/'nulo' exigem votee nulo.
    CHECK ((kind = 'candidato') = (votee_elector_id IS NOT NULL))
);
CREATE INDEX IF NOT EXISTS idx_vote_round ON vote(round_id);
