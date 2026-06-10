# Sem placar parcial durante Escrutínio aberto

Enquanto um Escrutínio está aberto, o Telão mostra apenas o **progresso**
(quantos Delegados já votaram de quantos presentes), **nunca a contagem por
candidato**. O placar só aparece depois que a Mesa encerra o Escrutínio.

**Por quê:** em voto secreto, expor o parcial induz efeito manada — os últimos a
votar mudam a escolha ao ver quem está ganhando. Num Congresso pequeno, poucos
votos definem a tendência, então o efeito é forte e compromete a lisura.

**Consequência:** não exponha endpoints/UI de contagem por candidato de um
Escrutínio cujo status seja "aberto". Progresso (total votado) é permitido. Quem
sentir falta de "transparência em tempo real" deve ser respondido com a
Reconciliação pós-encerramento, que é auditável e não influencia o voto.
