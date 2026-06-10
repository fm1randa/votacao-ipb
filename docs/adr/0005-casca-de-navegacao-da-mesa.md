# Casca de navegação da mesa + escrutínio de cargo único

A área da mesa deixa de ser uma página-rolo única e passa a uma **casca com
navegação persistente**: **barra lateral** no desktop, **barra inferior** (estilo
app nativo) no mobile, com 4 abas — **Escrutínio · Credenciamento · Telão ·
Relatório**. A área do eleitor permanece **linear** (sem abas; "Início" no topo).

A aba **Escrutínio** mostra só o **cargo da vez + uma ação** que muda de estado
(Abrir → Encerrar → Abrir próximo → avança sozinha ao decidir), com uma tira de
progresso "Cargo N de 6". O **Credenciamento** é página dedicada com busca (nome ou
igreja) e filtros rápidos (pendentes/presentes/ausentes).

**Por quê:** a versão anterior despejava os 6 cargos e o rol inteiro numa só página
("mobile esticado" no desktop), confusa para operador leigo. Como a **ordem dos
cargos é fixa**, listar todos é ruído — basta o cargo atual.

**Consequência:** novas rotas `/board/credenciamento` e `/board/telao`; `/board`
vira a aba Escrutínio. Layout responsivo real (sidebar ≥900px / barra inferior
abaixo), conteúdo em coluna única alargada (sem multi-coluna).
