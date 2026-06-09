package workflows

import (
	"context"
	"log/slog"
	"testing"
)

func TestRunN8NReplacementBuildsLeadAndClickUpFields(t *testing.T) {
	runner := NewRunner(slog.Default(), Config{})

	output, err := runner.RunN8NReplacement(context.Background(), N8NReplacementInput{
		EventID: "evt_123",
		Body: webhookBody{
			Respondent: webhookRespondent{
				Answers: map[string]any{
					"Qual é o seu nome completo?":                                                          "Maria Silva",
					"Qual é o seu telefone (com WhatsApp)?":                                                "(65) 99999-0000",
					"Para qual concurso deseja a mentoria?":                                                "MPT",
					"Qual é o seu e-mail?":                                                                 "maria@example.com",
					"Qual é a sua área de graduação/curso de formação superior?":                           "Direito",
					"Qual é a sua atual situação profissional?":                                            "CLT",
					"Há quanto tempo estuda para concursos públicos?":                                      "Mais de 1 ano",
					"Já prestou provas de concursos públicos? Quais foram seus resultados? ":               "Já prestei, mas sem aprovação",
					"Quantas horas por semana você pode se dedicar aos estudos?":                           "Entre 40 e 60 horas",
					"Qual a sua maior necessidade em uma mentoria ?":                                       "Cronograma, método e disciplina",
					"O quanto você está comprometido com sua aprovação? ":                                  "100% comprometido",
					"Qual é a sua média de renda atual?":                                                   "Entre 3 e 5k",
					"Se for selecionado, você estaria disposto e teria condições de investir na mentoria?": "Sim, tenho condições",
					"Como conheceu a nossa mentoria?":                                                      "Instagram",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("RunN8NReplacement returned error: %v", err)
	}

	if output.LeadClass != "0" {
		t.Fatalf("expected hot lead class 0, got %q with score %d", output.LeadClass, output.LeadScore)
	}
	if output.PhoneE164 != "+5565999990000" {
		t.Fatalf("unexpected phone: %q", output.PhoneE164)
	}
	if output.ContestCode == nil || *output.ContestCode != 2 {
		t.Fatalf("unexpected contest code: %#v", output.ContestCode)
	}
	if output.Name != "Maria Silva" {
		t.Fatalf("unexpected name: %q", output.Name)
	}
	if output.ClickUpSubmitted {
		t.Fatal("clickup should be skipped without credentials")
	}
	if len(output.CustomFields) != 11 {
		t.Fatalf("expected 11 custom fields, got %d", len(output.CustomFields))
	}
}

func TestNormalizePhone(t *testing.T) {
	tests := map[string]string{
		"(65) 99999-0000":   "+5565999990000",
		"+55 65 99999-0000": "+5565999990000",
		"":                  "",
	}

	for input, expected := range tests {
		if got := normalizePhone(input); got != expected {
			t.Fatalf("normalizePhone(%q) = %q, want %q", input, got, expected)
		}
	}
}

func TestRunN8NReplacementAcceptsNormalizedQuestionKeys(t *testing.T) {
	runner := NewRunner(slog.Default(), Config{})

	output, err := runner.RunN8NReplacement(context.Background(), N8NReplacementInput{
		Answers: map[string]any{
			"Nome completo":                 "Joao Teste",
			"Telefone com WhatsApp":         "65 99999-0000",
			"Concurso alvo":                 "mpt",
			"Email":                         "joao@example.com",
			"Area de graduacao":             "Direito",
			"Situacao profissional":         "clt",
			"Tempo de estudo":               "mais de 1 ano",
			"Historico de provas":           "prestei sem aprovação",
			"Horas disponiveis por semana":  "entre 40 e 60 horas",
			"Maior necessidade na mentoria": "metodo e disciplina",
			"Comprometimento":               "100% comprometido",
			"Renda atual":                   "entre 3 e 5k",
			"Disposicao para investir":      "sim, tenho condicoes",
			"Origem":                        "instagram",
		},
	})
	if err != nil {
		t.Fatalf("RunN8NReplacement returned error: %v", err)
	}

	if output.Name != "Joao Teste" {
		t.Fatalf("unexpected name: %q", output.Name)
	}
	if output.PhoneE164 != "+5565999990000" {
		t.Fatalf("unexpected phone: %q", output.PhoneE164)
	}
	if output.ContestCode == nil || *output.ContestCode != 2 {
		t.Fatalf("unexpected contest code: %#v", output.ContestCode)
	}
	if output.LeadClass != "0" {
		t.Fatalf("expected lead class 0, got %q with score %d", output.LeadClass, output.LeadScore)
	}
	if len(output.CustomFields) != 11 {
		t.Fatalf("expected 11 custom fields, got %d", len(output.CustomFields))
	}
}

func TestTaskMatchesLeadIdentity(t *testing.T) {
	task := clickUpTaskDetails{
		ID: "task_123",
		CustomFields: []clickUpCustomField{
			{ID: emailCustomFieldID, Value: "Lead@Example.com"},
			{ID: "6bc1d844-7e9d-4640-b8c8-97a54a9aea12", Value: "(65) 99999-0000"},
		},
	}

	if !taskMatchesLeadIdentity(task, leadIdentity{Email: "lead@example.com"}) {
		t.Fatal("expected task to match by normalized email")
	}
	if !taskMatchesLeadIdentity(task, leadIdentity{PhoneE164: "+5565999990000"}) {
		t.Fatal("expected task to match by normalized phone")
	}
	if taskMatchesLeadIdentity(task, leadIdentity{Email: "other@example.com", PhoneE164: "+5565888880000"}) {
		t.Fatal("did not expect task to match different identity")
	}
}
