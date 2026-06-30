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

### Webhook de negocio fechado

Recebe eventos com `payload.id` apontando para uma task do ClickUp, busca a task original, cria uma task de onboarding e copia o e-mail para a nova task.

```bash
curl -X POST http://localhost:8080/mentoria/webhooks/negocio-fechado \
  -H 'Content-Type: application/json' \
  -d '{"payload":{"id":"86afkfpwd"}}'
```

Tambem existem aliases para o path original do N8N:

```text
POST /NEGOCIOFECHADO
POST /mentoria/NEGOCIOFECHADO
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
export ONBOARDING_LIST_ID=...
```

Sem essas variaveis, o servidor processa o lead e retorna o payload montado, mas nao envia nada ao ClickUp.

`CLICKUP_LIST_ID` e usado na captacao de leads. `ONBOARDING_LIST_ID` e usado pela automacao de negocio fechado para criar a task de onboarding.

Opcionalmente configure um responsavel padrao da task de onboarding:

```bash
export ONBOARDING_ASSIGNEE_ID=...
```

## Google Apps Script

Para enviar tambem para uma planilha/script externo, configure:

```bash
export GOOGLE_WEBHOOK_URL=...
```

O servidor envia um `POST` JSON com dados processados do lead e as respostas originais do formulario.

## Backup dos webhooks em Postgres

Para guardar todo payload recebido antes de chamar ClickUp/Google, configure:

```bash
export DATABASE_URL='postgres://mentoria:SENHA_FORTE@localhost:5432/mentoria?sslmode=disable'
```

Quando `DATABASE_URL` estiver configurada, o servidor cria automaticamente a tabela `webhook_events` e grava:

- workflow chamado
- path, metodo, headers e query string
- body bruto recebido
- status final (`processed`, `duplicate`, `failed` ou `invalid_json`)
- resposta ou erro do processamento

Exemplo local com Docker:

```bash
docker run -d \
  --name mentoria-postgres \
  --restart unless-stopped \
  -e POSTGRES_DB=mentoria \
  -e POSTGRES_USER=mentoria \
  -e POSTGRES_PASSWORD='SENHA_FORTE' \
  -v mentoria-postgres-data:/var/lib/postgresql/data \
  -p 5432:5432 \
  postgres:17-alpine
```

Na VPS, prefira deixar o Postgres sem porta publica e conectar os containers pela mesma rede Docker:

```bash
docker network create mentoria-net 2>/dev/null || true

docker run -d \
  --name mentoria-postgres \
  --restart unless-stopped \
  --network mentoria-net \
  -e POSTGRES_DB=mentoria \
  -e POSTGRES_USER=mentoria \
  -e POSTGRES_PASSWORD='SENHA_FORTE' \
  -v mentoria-postgres-data:/var/lib/postgresql/data \
  postgres:17-alpine
```

Depois rode o container da API na mesma rede com:

```bash
-e DATABASE_URL='postgres://mentoria:SENHA_FORTE@mentoria-postgres:5432/mentoria?sslmode=disable'
--network mentoria-net
```

Para auditar eventos recentes:

```bash
docker exec -it mentoria-postgres psql -U mentoria -d mentoria \
  -c "select id, workflow, status, http_status, created_at, error_message from webhook_events order by id desc limit 20;"
```

Para localizar payloads que chegaram mas falharam no processamento:

```bash
docker exec -it mentoria-postgres psql -U mentoria -d mentoria \
  -c "select id, workflow, raw_body from webhook_events where status = 'failed' order by id desc;"
```
