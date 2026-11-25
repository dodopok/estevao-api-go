<!--
Título: Conventional Commits em inglês: `feat(calendar): …`, `fix(easter): …`.
Escreva para quem vai revisar sem ter acompanhado a conversa. Seção que não se
aplica fica com "Nada." em vez de sumir: assim fica claro que ela foi pensada.
Estes comentários não aparecem no PR.
-->

## Por quê

<!-- O que este passo da prova de conceito quer responder ou medir em relação
à estevao-api (Rails). Link para a issue ou a conversa. -->

## O que muda

<!-- O que o POC passa a fazer: endpoint, cálculo, comparação. Lista curta; o
diff já mostra o código. -->

## Como foi feito

<!-- O que o diff não explica: a abordagem, o que foi descartado e por quê, os
arquivos que valem ser lidos primeiro. -->

## Riscos

<!--
Uma linha por risco: o que pode dar errado, quem sente, e o que segura. Pense
em:
- divergência com a estevao-api, que é a referência: formato da resposta,
  regra litúrgica, livro de oração, data;
- dado de produção ou segredo usado aqui (banco, chave, `.env`);
- alguém passando a chamar o POC como se fosse a API.
Se não há risco que valha nomear, diga por quê.
-->

## Deploy

<!--
Hoje nada é publicado a partir deste repositório. Se este PR passa a publicar
algo (servidor, imagem, binário), diga onde, como e quem chama.
-->

- [ ] Nada a publicar.

## Como desfazer

<!-- Reverter o merge basta enquanto nada é publicado. Diga o que não volta se
algo passou a rodar fora daqui. -->

## Como testar

<!-- O que foi rodado, com o resultado (build, formatação, testes), e a
comparação com a resposta da estevao-api para o mesmo endpoint, livro e
data. -->

## Relacionados

<!-- PRs em outros repositórios (`dodopok/estevao-api#123`) e issues. -->

## Checklist

- [ ] O que foi rodado (build, formatação, testes) está em "Como testar", com o resultado
- [ ] Resposta comparada com a da estevao-api para o mesmo endpoint, livro e data
- [ ] Regra litúrgica igual à do Rails, ou a diferença dita no PR
- [ ] Nenhuma chave, token ou `.env` no diff
- [ ] README diz o que o POC já cobre e como rodar
