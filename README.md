# Mentoria Automation Server

Servidor Go para substituir automacoes feitas no N8N por codigo versionado, testavel e facil de operar.

## Rodando localmente

```bash
go run ./cmd/server
```

Por padrao o servidor sobe em `:8080`. Para trocar:

```bash
HTTP_ADDR=:3000 go run ./cmd/server
```

## Endpoints

### Healthcheck

```bash
curl http://localhost:8080/healthz
```

### Webhook da automacao migrada

```bash
curl -X POST http://localhost:8080/webhooks/n8n-replacement \
  -H 'Content-Type: application/json' \
  -d '{
    "event_id": "evt_123",
    "body": {
      "respondent": {
        "answers": {
          "Qual é o seu nome completo?": "Maria Silva",
          "Qual é o seu telefone (com WhatsApp)?": "(65) 99999-0000",
          "Para qual concurso deseja a mentoria?": "MPT",
          "Há quanto tempo estuda para concursos públicos?": "Mais de 1 ano",
          "Já prestou provas de concursos públicos? Quais foram seus resultados? ": "Já prestei, mas sem aprovação",
          "Quantas horas por semana você pode se dedicar aos estudos?": "Entre 40 e 60 horas",
          "Qual a sua maior necessidade em uma mentoria ?": "Cronograma, método e disciplina",
          "O quanto você está comprometido com sua aprovação? ": "100% comprometido",
          "Se for selecionado, você estaria disposto e teria condições de investir na mentoria?": "Sim, tenho condições"
        }
      }
    }
  }'
```

## Onde implementar a automacao

A logica que substitui o fluxo do N8N fica em:

```text
internal/workflows/n8n_replacement.go
```

O fluxo atual faz:

- extracao das respostas do formulario (`body.respondent.answers`, `respondent.answers` ou `answers`)
- classificacao do lead como `0`, `1` ou `2`
- normalizacao do telefone para E.164 com fallback Brasil (`+55`)
- mapeamento do concurso alvo para codigo
- montagem dos custom fields do ClickUp
- criacao da task no ClickUp quando as variaveis abaixo estiverem definidas
- envio opcional para Google Apps Script quando `GOOGLE_WEBHOOK_URL` estiver definida

## ClickUp

Configure:

```bash
export CLICKUP_TOKEN=...
export CLICKUP_LIST_ID=...
```

Sem essas variaveis, o servidor processa o lead e retorna o payload montado, mas nao envia nada ao ClickUp.

## Google Apps Script

Para enviar tambem para uma planilha/script externo, configure:

```bash
export GOOGLE_WEBHOOK_URL=...
```

O servidor envia um `POST` JSON com dados processados do lead e as respostas originais do formulario.
