-- Schema do sistema de votação (Congresso de Federação UMP).
-- Base normativa: GTSI 2015, Específica UMP (Art. 26, 49–52, 90–91). Ver SPEC.md.
--
-- Princípios centrais:
--  * Sigilo: `vote` referencia o delegado VOTADO (público), nunca o votante.
--    O elo do votante é só o `token` cego, sorteado às cegas no credenciamento.
--  * Presença é da PESSOA, não do token (ADR-0002): quórum/reconciliação contam
--    `elector.presente` (reversível), nunca tokens entregues.
--  * Queima atômica do token: UNIQUE(round_id, token) em `vote`.

PRAGMA foreign_keys = ON;

-- Configurações simples chave/valor (ex.: hash do PIN da Mesa, definido na 1ª vez).
CREATE TABLE IF NOT EXISTS setting (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

-- O evento. Em geral há um só por banco.
CREATE TABLE IF NOT EXISTS congress (
    id                INTEGER PRIMARY KEY,
    federacao         TEXT    NOT NULL,
    ano               INTEGER NOT NULL,
    quorum_declarado  INTEGER NOT NULL DEFAULT 0,  -- gate: a Mesa declara o quórum
    encerrada         INTEGER NOT NULL DEFAULT 0,  -- eleição encerrada (só leitura)
    criado_em         TEXT    NOT NULL DEFAULT (datetime('now'))
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

-- UMP local: base do quórum por representação (Art. 49a).
CREATE TABLE IF NOT EXISTS local (
    id           INTEGER PRIMARY KEY,
    congress_id  INTEGER NOT NULL REFERENCES congress(id) ON DELETE CASCADE,
    nome         TEXT    NOT NULL
);

-- Delegado (rol). `nato` => sem credencial e sem UMP local (Art. 52).
-- `credenciado` é monótono (já recebeu token alguma vez); `presente` é reversível
-- (a Mesa registra saída/reentrada) e é o que conta para quórum/reconciliação.
CREATE TABLE IF NOT EXISTS elector (
    id           INTEGER PRIMARY KEY,
    congress_id  INTEGER NOT NULL REFERENCES congress(id) ON DELETE CASCADE,
    nome         TEXT    NOT NULL,
    local_id     INTEGER REFERENCES local(id),   -- nulo para nato
    nato         INTEGER NOT NULL DEFAULT 0,
    nascimento   TEXT,                            -- opcional; desempate por idade (Art. 91g)
    credenciado  INTEGER NOT NULL DEFAULT 0,
    presente     INTEGER NOT NULL DEFAULT 0,
    criado_em    TEXT    NOT NULL DEFAULT (datetime('now'))
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

-- Cargo: posto eletivo da Diretoria, eleito em sequência (Art. 26a).
-- `ativo`: federações menores podem desabilitar Vice/Sec.Exec/2º Sec (SPEC §3.5).
CREATE TABLE IF NOT EXISTS position (
    id                 INTEGER PRIMARY KEY,
    congress_id        INTEGER NOT NULL REFERENCES congress(id) ON DELETE CASCADE,
    nome               TEXT    NOT NULL,
    seq                INTEGER NOT NULL,
    ativo              INTEGER NOT NULL DEFAULT 1,
    status             TEXT    NOT NULL DEFAULT 'pendente'
                       CHECK (status IN ('pendente', 'em_eleicao', 'decidido')),
    eleito_elector_id  INTEGER REFERENCES elector(id),
    UNIQUE (congress_id, seq)
);

-- Escrutínio: rodada 1..3 de um cargo. `runoff`=1 no 3º (entre os 2 mais votados).
CREATE TABLE IF NOT EXISTS round (
    id            INTEGER PRIMARY KEY,
    position_id   INTEGER NOT NULL REFERENCES position(id) ON DELETE CASCADE,
    numero        INTEGER NOT NULL,             -- 1, 2 ou 3
    status        TEXT    NOT NULL DEFAULT 'aberto'
                  CHECK (status IN ('aberto', 'encerrado')),
    runoff        INTEGER NOT NULL DEFAULT 0,
    aberto_em     TEXT    NOT NULL DEFAULT (datetime('now')),
    encerrado_em  TEXT,
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
-- A UNIQUE(round_id, token) é a queima do token (impede voto duplo no escrutínio).
CREATE TABLE IF NOT EXISTS vote (
    id                INTEGER PRIMARY KEY,
    round_id          INTEGER NOT NULL REFERENCES round(id) ON DELETE CASCADE,
    token             TEXT    NOT NULL REFERENCES token(token),
    kind              TEXT    NOT NULL CHECK (kind IN ('candidato', 'branco', 'nulo')),
    votee_elector_id  INTEGER REFERENCES elector(id),
    criado_em         TEXT    NOT NULL DEFAULT (datetime('now')),
    UNIQUE (round_id, token),
    -- 'candidato' exige votee; 'branco'/'nulo' exigem votee nulo.
    CHECK ((kind = 'candidato') = (votee_elector_id IS NOT NULL))
);
CREATE INDEX IF NOT EXISTS idx_vote_round ON vote(round_id);
