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
	GoogleSubmitted  bool          `json:"google_submitted"`
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
	customFields := buildClickUpCustomFields(answers, phone, contestCode, leadClass)
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
	} else {
		task, err := r.createClickUpTask(ctx, name, leadClass, customFields)
		if err != nil {
			return N8NReplacementOutput{}, err
		}
		output.ClickUpSubmitted = true
		output.ClickUpTaskID = task.ID
		output.ClickUpTaskURL = task.URL
	}

	if r.config.GoogleWebhookURL == "" {
		r.logger.Info("google webhook skipped: missing GOOGLE_WEBHOOK_URL")
		return output, nil
	}

	if err := r.submitGoogleWebhook(ctx, output, answers); err != nil {
		return N8NReplacementOutput{}, err
	}
	output.GoogleSubmitted = true
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

func buildClickUpCustomFields(answers map[string]any, phone string, code *int, leadClass string) []CustomField {
	fields := []CustomField{
		{ID: "6bc1d844-7e9d-4640-b8c8-97a54a9aea12", Value: emptyToNil(phone)},
		{ID: "942ba052-d74c-4304-ab97-c0e35621f39c", Value: whatsappURL(phone)},
		{ID: "2861d5b0-9415-4d01-bfce-c79ef12a3d3f", Value: contestOptionID(code)},
		{ID: "d42c2f67-4cc1-4c5b-b6ee-ab13c75639c0", Value: leadTemperatureOptionID(leadClass)},
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
	if len(bytes.TrimSpace(responseBody)) == 0 {
		return clickUpTask{}, nil
	}

	var task clickUpTask
	if err := json.Unmarshal(responseBody, &task); err != nil {
		if errors.Is(err, io.EOF) || strings.Contains(err.Error(), "unexpected end of JSON input") {
			return clickUpTask{}, nil
		}
		return clickUpTask{}, err
	}
	return task, nil
}

type clickUpTask struct {
	ID  string `json:"id"`
	URL string `json:"url"`
}

func (r *Runner) submitGoogleWebhook(ctx context.Context, output N8NReplacementOutput, answers map[string]any) error {
	payload := map[string]any{
		"workflow":         output.Workflow,
		"event_id":         output.EventID,
		"status":           output.Status,
		"lead_class":       output.LeadClass,
		"lead_score":       output.LeadScore,
		"name":             output.Name,
		"phone_e164":       output.PhoneE164,
		"contest_code":     output.ContestCode,
		"custom_fields":    output.CustomFields,
		"clickup_task_id":  output.ClickUpTaskID,
		"clickup_task_url": output.ClickUpTaskURL,
		"handled_at":       output.Handled,
		"answers":          answers,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.config.GoogleWebhookURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	responseBody, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("google webhook returned %s: %s", res.Status, string(responseBody))
	}

	return nil
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

func mappedValue(value string, mapping map[string]string) any {
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
	return "https://wa.me/" + strings.TrimPrefix(phone, "+")
}

func contestOptionID(code *int) any {
	if code == nil {
		return nil
	}
	options := map[int]string{
		0: "7fb2c0de-3775-44e5-ad14-d9ffbdeb6674",
		1: "7a030cae-dc4e-4c08-b54d-bac6c1edaec8",
		2: "1154bd08-b80e-4c39-a153-b67fbab16403",
		3: "b1631d94-cf5d-4f5c-b372-06b356bf1532",
	}
	return options[*code]
}

func leadTemperatureOptionID(leadClass string) any {
	options := map[string]string{
		"0": "b57d5d57-3d76-4dfd-8574-7b3b9f811281",
		"1": "8245a466-a965-4ec8-b0d2-d52e215ad707",
		"2": "838626a2-6fff-4a71-bc24-aa4fa683cb5d",
	}
	return options[leadClass]
}

func situationMap() map[string]string {
	return map[string]string{"Desempregado": "fdb7fb4f-1cae-4162-be24-fba8ac05db56", "Sou desempregado": "fdb7fb4f-1cae-4162-be24-fba8ac05db56", "Estou desempregado": "fdb7fb4f-1cae-4162-be24-fba8ac05db56", "Estagiário": "3b13ab2e-3fbc-4db4-b775-1efaa43c96a3", "Sou estagiário": "3b13ab2e-3fbc-4db4-b775-1efaa43c96a3", "Autônomo": "2bae7032-b8a5-4133-8713-e6e4907240e2", "Sou autônomo": "2bae7032-b8a5-4133-8713-e6e4907240e2", "CLT": "73ee0ddd-67d8-4cf9-9d51-c8038349f61f", "Sou CLT": "73ee0ddd-67d8-4cf9-9d51-c8038349f61f", "Trabalho com carteira assinada": "73ee0ddd-67d8-4cf9-9d51-c8038349f61f", "Servidor Publico": "226635e4-6e84-4652-891f-8e3d3bf80276", "Servidor público": "226635e4-6e84-4652-891f-8e3d3bf80276", "Sou servidor público": "226635e4-6e84-4652-891f-8e3d3bf80276", "Já sou servidor público": "226635e4-6e84-4652-891f-8e3d3bf80276", "Outro": "d4f76b26-7585-4334-badf-59a570f0e17a", "Outros": "d4f76b26-7585-4334-badf-59a570f0e17a"}
}

func studyTimeMap() map[string]string {
	return map[string]string{"Ainda não comecei": "dfa855d5-8dec-4712-8e2e-d6fa2980c4f0", "Há menos de 6 meses": "acbd635f-9388-4a4b-8521-ce380e15c185", "Entre 6 meses e 1 ano": "7ae17340-9a51-4533-bdc1-043786967141", "Entre 6 meses a 1 ano": "7ae17340-9a51-4533-bdc1-043786967141", "mais de 1 ano": "ac9db1a8-8df8-4f71-bf28-0baff8402262", "Mais de 1 ano": "ac9db1a8-8df8-4f71-bf28-0baff8402262"}
}

func incomeMap() map[string]string {
	return map[string]string{"Menos de 2 mil": "380a2865-0335-4432-a198-41336407563a", "Menos de R$2.000,00": "380a2865-0335-4432-a198-41336407563a", "Entre 2 e 3k": "de19fc65-d210-49ba-9eb2-273bf61200cf", "Entre R$2.000,00 e R$3.000,00": "de19fc65-d210-49ba-9eb2-273bf61200cf", "Entre 3 e 5k": "ccc48ef5-2010-4f03-9872-ede5cffcf152", "Entre R$3.000,00 e R$5.000,00": "ccc48ef5-2010-4f03-9872-ede5cffcf152", "mais de 5k": "40313ac9-0996-4116-b973-879a984b2a02", "Mais de R$5.000,00": "40313ac9-0996-4116-b973-879a984b2a02", "Prefiro não dizer": "91a17151-28bd-4cd9-b593-4bf944d9bf40"}
}

func weeklyHoursMap() map[string]string {
	return map[string]string{"Até 10 horas": "b588ee97-2301-47b5-be92-1f6ab59211be", "Menos de 10 horas": "b588ee97-2301-47b5-be92-1f6ab59211be", "Entre 10 e 20 horas": "b588ee97-2301-47b5-be92-1f6ab59211be", "Entre 20 e 30 horas": "0f3474b1-0649-4244-9785-2680b24b6009", "Entre 40 e 60 horas": "a8846d23-d833-4762-863e-fbcc4f43f944", "Mais de 60 horas": "a6a89b3f-3abf-4247-b9eb-7d54901160ff"}
}

func originMap() map[string]string {
	return map[string]string{"Aluno": "2f04fa93-f02d-458c-831f-5980f7734394", "Ja fui aluno(a)": "2f04fa93-f02d-458c-831f-5980f7734394", "Já fui aluno(a)": "2f04fa93-f02d-458c-831f-5980f7734394", "Instagram": "3d92fd0c-6cd8-43fc-b718-8b788f989c94", "Pelo Instagram": "3d92fd0c-6cd8-43fc-b718-8b788f989c94", "Youtube": "833c6577-f8d0-4850-a4a1-0e13fb955000", "Pelo Youtube": "833c6577-f8d0-4850-a4a1-0e13fb955000", "Indicação": "2922f5f4-ecae-4fd6-8d3d-a5b47d12b811", "Indicação de um dos mentorados": "2922f5f4-ecae-4fd6-8d3d-a5b47d12b811", "Indicação de amigos": "2922f5f4-ecae-4fd6-8d3d-a5b47d12b811", "Google": "60342ece-feb8-4ba0-a76f-a76ed1db5768", "Pelo Google": "60342ece-feb8-4ba0-a76f-a76ed1db5768"}
}
