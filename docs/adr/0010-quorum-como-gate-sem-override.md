# Quórum computado como gate da abertura, sem override

O quórum deixa de ser uma **declaração manual** da mesa (o antigo botão
"declarar quórum" sobre `quorum_declarado`) e passa a ser **computado pelo
sistema e imposto como bloqueio**: o botão **"Declarar abertura"** (Art. 56c)
só habilita quando o quórum do âmbito está atingido — Local: mais da metade dos
sócios ativos do rol; Federação: mais da metade das UMPs locais; Sinodal: mais
da metade das Federações; Nacional: mais da metade das Sinodais **e** pelo menos
um terço das Federações (Art. 12 §2º, 49).

**Quando o gate vale:** **só na abertura**, fiel ao GTSI (a verificação de
quórum precede a Declaração de Abertura — Art. 56c; a plenária local "só poderá
funcionar" com metade dos sócios — Art. 12 §2º). Depois de aberta a eleição,
escrutínios abrem sem nova exigência: se delegados de 4 UMPs saem pro almoço, a
eleição do Tesoureiro não trava. O painel continua mostrando a contagem ao vivo.

**Sem botão de override.** A válvula de escape é **corrigir os dados**: se o
denominador está errado (ex.: rol local com sócios já desligados pelo Art. 8º),
a mesa edita o rol — ação que já existe, já passa pelo log de operações
(ADR-0006) e é restaurável. Um "declarar mesmo assim" reabriria exatamente a
porta que este ADR fecha: eleição iniciada sem quórum demonstrável. O custo
aceito é que a mesa precisa curar o rol na hora, sob pressão do evento — mas
cada remoção fica registrada e reversível no Histórico, o que é mais auditável
que uma justificativa em texto livre.

**Consequência para a Nacional:** computar o critério composto do Art. 49c
exige que o rol nacional registre Sinodal e Federação de cada delegado
(hierarquia de dois níveis — ver ADR-0009).

**Consequência para o local:** o rol importado precisa ser o rol **completo** de
sócios ativos, não só os esperados no evento — ele é o denominador do quórum.
