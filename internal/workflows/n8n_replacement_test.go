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
	if len(output.CustomFields) != 10 {
		t.Fatalf("expected 10 custom fields, got %d", len(output.CustomFields))
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
