# Presença é da pessoa, não do Token

Presença (e, portanto, Quórum e Reconciliação) conta **Delegados presentes**
(registro público, equivalente ao Livro de Presença), **não tokens entregues**.
Credenciar é um passo com dois atos: marcar o Delegado presente E entregar um
Token cego. Reemitir um Token (perda) **não** altera a presença.

**Por quê:** o Token é cego de propósito (não sabemos qual foi pra quem), logo um
código perdido não pode ser revogado. Medir presença por pessoa permite reemitir
sem inflar o Quórum, e transforma a Reconciliação numa trava real: como
`depositados ≤ presentes (pessoas)`, um token perdido-e-achado que vote gera
`depositados > presentes` → alarme. Se presença fosse contada por token, os dois
subiriam juntos e o abuso passaria batido.

**Consequência:** o sistema **detecta**, não **previne**, voto duplo por token
extra (preveni-lo exigiria ligar token↔pessoa, o que quebraria o sigilo — vetado).
Em federação pequena e de confiança, detecção via Reconciliação é a troca aceita.
Tokens em circulação podem exceder os presentes pelo nº de reemissões.
