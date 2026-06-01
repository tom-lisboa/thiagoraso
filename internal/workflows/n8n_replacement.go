package workflows

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

type N8NReplacementInput struct {
	Body       webhookBody       `json:"body"`
	Respondent webhookRespondent `json:"respondent"`
	Answers    map[string]any    `json:"answers"`
	EventID    string            `json:"event_id"`
}

type N8NReplacementOutput struct {
	Workflow         string        `json:"workflow"`
	EventID          string        `json:"event_id,omitempty"`
	Status           string        `json:"status"`
	LeadClass        string        `json:"lead_class"`
	LeadScore        int           `json:"lead_score"`
	Name             string        `json:"name"`
	PhoneE164        string        `json:"phone_e164,omitempty"`
	ContestCode      *int          `json:"contest_code,omitempty"`
	CustomFields     []CustomField `json:"custom_fields"`
	ClickUpTaskID    string        `json:"clickup_task_id,omitempty"`
	ClickUpTaskURL   string        `json:"clickup_task_url,omitempty"`
	ClickUpSubmitted bool          `json:"clickup_submitted"`
	Handled          time.Time     `json:"handled_at"`
}

type webhookBody struct {
	Respondent webhookRespondent `json:"respondent"`
}

type webhookRespondent struct {
	Answers map[string]any `json:"answers"`
}

type CustomField struct {
	ID    string `json:"id"`
	Value any    `json:"value"`
}

func (r *Runner) RunN8NReplacement(ctx context.Context, input N8NReplacementInput) (N8NReplacementOutput, error) {
	if err := ctx.Err(); err != nil {
		return N8NReplacementOutput{}, err
	}

	answers := input.answerMap()
	if len(answers) == 0 {
		return N8NReplacementOutput{}, errors.New("respondent answers are required")
	}

	leadClass, score := classifyLead(answers)
	phone := normalizePhone(answerString(answers, "Qual é o seu telefone (com WhatsApp)?"))
	contestCode := contestCode(answerStringAny(answers,
		"Para qual concurso deseja a mentoria?",
		"Para qual concurso deseja uma mentoria ?",
		"Para qual concurso deseja uma mentoria?",
	))
	customFields := buildClickUpCustomFields(answers, phone, contestCode)
	name := strings.TrimSpace(answerString(answers, "Qual é o seu nome completo?"))
	if name == "" {
		name = "Lead"
	}

	output := N8NReplacementOutput{
		Workflow:     "n8n-replacement",
		EventID:      input.EventID,
		Status:       "processed",
		LeadClass:    leadClass,
		LeadScore:    score,
		Name:         name,
		PhoneE164:    phone,
		ContestCode:  contestCode,
		CustomFields: customFields,
		Handled:      time.Now().UTC(),
	}

	if r.config.ClickUpToken == "" || r.config.ClickUpListID == "" {
		r.logger.Info("clickup skipped: missing CLICKUP_TOKEN or CLICKUP_LIST_ID")
		return output, nil
	}

	task, err := r.createClickUpTask(ctx, name, leadClass, customFields)
	if err != nil {
		return N8NReplacementOutput{}, err
	}
	output.ClickUpSubmitted = true
	output.ClickUpTaskID = task.ID
	output.ClickUpTaskURL = task.URL
	return output, nil
}

func (input N8NReplacementInput) answerMap() map[string]any {
	switch {
	case len(input.Body.Respondent.Answers) > 0:
		return input.Body.Respondent.Answers
	case len(input.Respondent.Answers) > 0:
		return input.Respondent.Answers
	default:
		return input.Answers
	}
}

func (r *Runner) VerifyMetaWebhook(values url.Values) (string, bool) {
	mode := values.Get("hub.mode")
	token := values.Get("hub.verify_token")
	challenge := values.Get("hub.challenge")

	if mode != "subscribe" || challenge == "" {
		return "", false
	}
	if r.config.MetaVerifyToken != "" && token != r.config.MetaVerifyToken {
		return "", false
	}
	return challenge, true
}

func classifyLead(answers map[string]any) (string, int) {
	score := 0
	score += scoreFromMap(answerString(answers, "Há quanto tempo estuda para concursos públicos?"), map[string]int{
		"mais de 1 ano": 20, "há mais de 1 ano": 20,
		"entre 6 meses e 1 ano": 15, "entre 6 meses a 1 ano": 15,
		"há menos de 6 meses": 10, "menos de 6 meses": 10,
		"ainda não comecei": 3,
	})
	score += scoreFromText(answerString(answers, "Já prestou provas de concursos públicos? Quais foram seus resultados? "), []textScore{
		{[]string{"aprov", "cadastro reserva", "fase"}, 15},
		{[]string{"já prestei", "prestei", "sem aprovação", "não fui aprovado"}, 10},
		{[]string{"nunca"}, 5},
	}, 5)
	score += scoreFromMap(answerString(answers, "Quantas horas por semana você pode se dedicar aos estudos?"), map[string]int{
		"mais de 60 horas": 15, "entre 40 e 60 horas": 15,
		"entre 20 e 30 horas": 10, "entre 10 e 20 horas": 5, "até 10 horas": 5, "menos de 10 horas": 5,
	})
	score += scoreFromText(answerString(answers, "O quanto você está comprometido com sua aprovação? "), []textScore{
		{[]string{"100%", "totalmente", "muito comprometido"}, 20},
		{[]string{"comprometido"}, 10},
		{[]string{"avaliando", "dúvida", "duvida"}, 5},
	}, 5)
	score += scoreFromText(answerString(answers, "Se for selecionado, você estaria disposto e teria condições de investir na mentoria?"), []textScore{
		{[]string{"tenho condições", "tenho condicoes", "sim"}, 15},
		{[]string{"organizar", "parcelar"}, 10},
		{[]string{"talvez"}, 5},
		{[]string{"não", "nao"}, 0},
	}, 5)
	score += scoreFromText(answerString(answers, "Qual a sua maior necessidade em uma mentoria ?"), []textScore{
		{[]string{"cronograma", "método", "metodo", "disciplina"}, 15},
		{[]string{"organização", "organizacao", "direcionamento", "estratégia", "estrategia"}, 10},
	}, 5)

	switch {
	case score >= 70:
		return "0", score
	case score >= 45:
		return "1", score
	default:
		return "2", score
	}
}

type textScore struct {
	terms []string
	score int
}

func scoreFromMap(value string, scores map[string]int) int {
	normalized := normalizeText(value)
	if score, ok := scores[normalized]; ok {
		return score
	}
	return 0
}

func scoreFromText(value string, scores []textScore, fallback int) int {
	normalized := normalizeText(value)
	for _, option := range scores {
		for _, term := range option.terms {
			if strings.Contains(normalized, normalizeText(term)) {
				return option.score
			}
		}
	}
	return fallback
}

func buildClickUpCustomFields(answers map[string]any, phone string, code *int) []CustomField {
	fields := []CustomField{
		{ID: "6bc1d844-7e9d-4640-b8c8-97a54a9aea12", Value: emptyToNil(phone)},
		{ID: "942ba052-d74c-4304-ab97-c0e35621f39c", Value: whatsappURL(phone)},
		{ID: "2861d5b0-9415-4d01-bfce-c79ef12a3d3f", Value: codeValue(code)},
		{ID: "0d4f8ffe-adeb-404d-a47d-8e0c8f9bdc6a", Value: emptyToNil(answerString(answers, "Qual é o seu e-mail?"))},
		{ID: "f4f0d81b-1408-4e03-a166-99fb8d7cd058", Value: emptyToNil(answerString(answers, "Qual é a sua área de graduação/curso de formação superior?"))},
		{ID: "5b4cb720-d577-4024-b193-7ab6a0161c00", Value: mappedValue(answerString(answers, "Qual é a sua atual situação profissional?"), situationMap())},
		{ID: "ffd42259-7bc2-4aa2-bc36-51f7a1c2dbab", Value: mappedValue(answerString(answers, "Há quanto tempo estuda para concursos públicos?"), studyTimeMap())},
		{ID: "38c2cfd3-f89f-44be-9d9a-6472cd5fa04e", Value: mappedValue(answerString(answers, "Qual é a sua média de renda atual?"), incomeMap())},
		{ID: "d75bc3e5-1005-4be8-a18b-28fb4444fb4a", Value: mappedValue(answerString(answers, "Quantas horas por semana você pode se dedicar aos estudos?"), weeklyHoursMap())},
		{ID: "6d3e3ff0-57bd-4bcb-a4f9-c2f05720985c", Value: mappedValue(answerString(answers, "Como conheceu a nossa mentoria?"), originMap())},
	}

	customFields := make([]CustomField, 0, len(fields))
	for _, field := range fields {
		if field.Value != nil {
			customFields = append(customFields, field)
		}
	}
	return customFields
}

func (r *Runner) createClickUpTask(ctx context.Context, name string, leadClass string, customFields []CustomField) (clickUpTask, error) {
	payload := map[string]any{
		"name":          name,
		"description":   "Lead classificado automaticamente. Classe: " + leadClass,
		"custom_fields": customFields,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return clickUpTask{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.clickup.com/api/v2/list/"+r.config.ClickUpListID+"/task", bytes.NewReader(body))
	if err != nil {
		return clickUpTask{}, err
	}
	req.Header.Set("Authorization", r.config.ClickUpToken)
	req.Header.Set("Content-Type", "application/json")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return clickUpTask{}, err
	}
	defer res.Body.Close()

	responseBody, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return clickUpTask{}, fmt.Errorf("clickup returned %s: %s", res.Status, string(responseBody))
	}

	var task clickUpTask
	if err := json.Unmarshal(responseBody, &task); err != nil {
		return clickUpTask{}, err
	}
	return task, nil
}

type clickUpTask struct {
	ID  string `json:"id"`
	URL string `json:"url"`
}

func answerString(answers map[string]any, key string) string {
	return answerStringAny(answers, key)
}

func answerStringAny(answers map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := answers[key]; ok {
			return stringifyAnswer(value)
		}
	}
	return ""
}

func stringifyAnswer(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case map[string]any:
		if country, ok := typed["country"].(string); ok {
			if phone, ok := typed["phone"].(string); ok {
				return country + phone
			}
		}
		bytes, _ := json.Marshal(typed)
		return string(bytes)
	default:
		return fmt.Sprint(value)
	}
}

var nonDigits = regexp.MustCompile(`\D+`)

func normalizePhone(raw string) string {
	phone := nonDigits.ReplaceAllString(raw, "")
	if phone == "" {
		return ""
	}
	if strings.HasPrefix(phone, "55") {
		return "+" + phone
	}
	return "+55" + phone
}

func contestCode(value string) *int {
	codes := map[string]int{
		"Auditor Fiscal do Trabalho - AFT": 0,
		"Tribunais do Trabalho (TRT/TST)":  1,
		"MPT":                              2,
		"Magistratura do Trabalho":         3,
	}
	if code, ok := codes[value]; ok {
		return &code
	}
	return nil
}

func mappedValue(value string, mapping map[string]int) any {
	if mapped, ok := mapping[value]; ok {
		return mapped
	}
	if mapped, ok := mapping[normalizeText(value)]; ok {
		return mapped
	}
	return nil
}

func normalizeText(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func emptyToNil(value string) any {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return value
}

func whatsappURL(phone string) any {
	if phone == "" {
		return nil
	}
	return "wa.me/" + phone
}

func codeValue(code *int) any {
	if code == nil {
		return nil
	}
	return *code
}

func situationMap() map[string]int {
	return map[string]int{"Desempregado": 0, "Sou desempregado": 0, "Estou desempregado": 0, "Estagiário": 1, "Sou estagiário": 1, "Autônomo": 2, "Sou autônomo": 2, "CLT": 3, "Sou CLT": 3, "Trabalho com carteira assinada": 3, "Servidor Publico": 4, "Servidor público": 4, "Sou servidor público": 4, "Já sou servidor público": 4, "Outro": 5, "Outros": 5, "Estudando": 6, "Estudando ": 6, "Sou estudante": 6, "Estou estudando": 6}
}

func studyTimeMap() map[string]int {
	return map[string]int{"Ainda não comecei": 0, "Há menos de 6 meses": 1, "Entre 6 meses e 1 ano": 2, "Entre 6 meses a 1 ano": 2, "mais de 1 ano": 3, "Mais de 1 ano": 3}
}

func incomeMap() map[string]int {
	return map[string]int{"Menos de 2 mil": 0, "Menos de R$2.000,00": 0, "Entre 2 e 3k": 1, "Entre R$2.000,00 e R$3.000,00": 1, "Entre 3 e 5k": 2, "Entre R$3.000,00 e R$5.000,00": 2, "mais de 5k": 3, "Mais de R$5.000,00": 3, "Prefiro não dizer": 4}
}

func weeklyHoursMap() map[string]int {
	return map[string]int{"Até 10 horas": 0, "Menos de 10 horas": 0, "Entre 10 e 20 horas": 0, "Entre 20 e 30 horas": 1, "Entre 40 e 60 horas": 2, "Mais de 60 horas": 3}
}

func originMap() map[string]int {
	return map[string]int{"Aluno": 0, "Ja fui aluno(a)": 0, "Já fui aluno(a)": 0, "Instagram": 1, "Pelo Instagram": 1, "Youtube": 2, "Pelo Youtube": 2, "Indicação": 3, "Indicação de um dos mentorados": 3, "Indicação de amigos": 3, "Google": 4, "Pelo Google": 4, "TikTok": 5, "Pelo TikTok": 5}
}
