package updater

import (
	"devcourse-finder/catalog"
	"golang.org/x/net/html"
	"time"
)

var profileContracts = map[string]profileHeadings{
	"yandex":       {Audience: []string{"Кому подойдёт курс", "Кому подойдёт этот курс", "Для кого этот курс"}, Requirements: []string{"Что нужно знать, чтобы начать изучать Python?", "Что нужно знать перед началом обучения?"}, Outcomes: []string{"Чему вы научитесь", "Как вы станете Python-разработчиком"}},
	"netology":     {Audience: []string{"Кому подойдёт курс", "Кому подойдет курс", "Кому подойдёт обучение"}, Requirements: []string{"Что нужно знать перед началом", "Требования к поступлению"}, Outcomes: []string{"Чему вы научитесь", "Результаты обучения"}},
	"skillbox":     {Audience: []string{"Кому подойдёт этот курс", "Кому подойдёт курс", "Кому подойдет курс"}, Requirements: []string{"Что нужно знать перед началом", "Я никогда не программировал. У меня получится?"}, Outcomes: []string{"Чему вы научитесь", "Какие навыки вы получите"}},
	"skillfactory": {Audience: []string{"Кому подойдет курс", "Кому подойдёт курс", "Для кого этот курс"}, Requirements: []string{"Какие знания нужны для обучения?", "Что нужно знать перед началом обучения?"}, Outcomes: []string{"Чему вы научитесь на курсе", "Чему вы научитесь"}},
	"skypro":       {Audience: []string{"Кому подойдёт курс", "Кому подойдет курс"}, Requirements: []string{"Что нужно знать перед началом обучения?"}, Outcomes: []string{"Чему вы научитесь на курсе «Python-разработчик» с нуля"}},
}

func attachHTMLProfile(data []byte, c Candidate, r catalog.Course, o Observation, now time.Time) (catalog.Course, Observation) {
	if o.ProfileObserved {
		return r, o
	}
	h, ok := profileContracts[c.Adapter]
	if !ok {
		return r, o
	}
	p := profileFromSections(data, c, r.Language, now, h)
	// Skillfactory explicitly states audience in the verified course feature
	// artboard. Job-vacancy 'Requirements' blocks are intentionally not used.
	if c.Adapter == "skillfactory" && p.Audience == nil {
		doc := learningDocument(data)
		n := learningOne(doc, func(n *html.Node) bool {
			return classToken(n, "tn-atom") && nodeText(n) == "Подходит новичкам без опыта в IT"
		})
		if n != nil {
			p.Audience = profileTags(nodeText(n), learningProof(data, c.URL, ".tn-atom[course-audience=Подходит новичкам без опыта в IT]", now), "audience")
		}
	}
	return profileAttach(r, o, p)
}
