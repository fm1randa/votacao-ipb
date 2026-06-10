# Token de 4 caracteres (risco de brute-force aceito)

O Token usa **4 caracteres** do alfabeto de 31 símbolos (sem ambíguos), digitados
em 4 caixas estilo OTP. Sem rate-limiting.

**Contexto:** 31⁴ ≈ 924 mil combinações. Com ~dezenas de tokens entregues, um
atacante na mesma LAN poderia, via script, acertar um token válido e ainda não
usado em ~1 a cada ~18 mil tentativas — em tese, votar fraudulentamente num
escrutínio aberto. Com 6 caracteres (31⁶ ≈ 887M) o ataque seria inviável.

**Decisão:** mesmo assim, 4 caracteres — priorizando a facilidade de digitação por
delegados leigos/idosos. O ambiente é um congresso de igreja, fechado e de
confiança, sem incentivo a ataque; a janela de cada escrutínio é curta; a rede é
local; e a **Reconciliação detecta** over-voting (`depositados > presentes`).

**Consequência:** é detecção, não prevenção. Se o contexto mudar (evento maior,
rede aberta/exposta, histórico de disputa acirrada), revisar para **6+ caracteres**
ou adicionar **rate-limit + alerta de tentativas inválidas na Mesa**. A
Reconciliação ([ADR-0002](0002-presenca-desacoplada-do-token.md)) é a rede de proteção.
