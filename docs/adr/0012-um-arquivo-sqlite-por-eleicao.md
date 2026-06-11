# Um arquivo SQLite por Eleição

Para administrar várias Eleições, cada uma vive num arquivo SQLite próprio
dentro de uma pasta de dados; o Gerenciador de Eleições lista os arquivos e
troca a Eleição ativa reabrindo o Store a quente (sem reiniciar o processo).
Rejeitamos múltiplos congressos no mesmo banco — apesar de o esquema ter
`congress_id` em todas as tabelas — porque o log de operações (ADR-0006) é
global ao banco: cada snapshot serializa TODAS as tabelas de domínio, então
Desfazer/Restaurar numa eleição reverteria as outras juntas. Escopar o log por
congresso exigiria reescrever snapshot/restore e migrar o histórico; com um
arquivo por Eleição, o log já nasce isolado, e excluir/arquivar/copiar uma
Eleição é uma operação de arquivo (backup = copiar o .db).

Consequências: o `congress_id` nas tabelas segue como está (inofensivo, e
mantém o ON DELETE CASCADE útil); o servidor passa a receber a pasta de dados
em vez de um arquivo; o PIN da Mesa é propagado por cópia do hash ao criar uma
Eleição nova (na prática, um PIN só); a Eleição ativa é lembrada num meta na
pasta, fora dos bancos.
