package updater

func init() {
	profileContracts["htmlacademy"] = profileHeadings{Audience: []string{"Подходит ли вам этот курс"}, Requirements: []string{"Что нужно знать перед началом курса"}, Outcomes: []string{"Чему вы научитесь"}}
	profileContracts["purpleschool"] = profileHeadings{Audience: []string{"Если я не умею программировать, подойдет ли мне курс?"}, Requirements: []string{"Какие знания нужны для прохождения курса?"}, Outcomes: []string{"Чему вы научитесь"}}
	profileContracts["otus"] = profileHeadings{Audience: []string{"Для кого этот курс?"}, Requirements: []string{"Необходимые знания"}, Outcomes: []string{"Чему вы научитесь"}}
	profileContracts["hexlet"] = profileHeadings{Audience: []string{"Кому подойдёт курс"}, Requirements: []string{"Что нужно знать перед началом обучения"}, Outcomes: []string{"Чему вы научитесь"}}
	profileContracts["javarush"] = profileHeadings{Audience: []string{"Кому подходит Java Premium"}, Requirements: []string{"Что нужно знать перед Java Premium"}, Outcomes: []string{"Чему вы научитесь с Java Premium"}}
	profileContracts["rsschool"] = profileHeadings{Audience: []string{"For everyone"}, Requirements: []string{"For everyone"}, Outcomes: []string{"What you will learn"}}
	profileContracts["codebasics"] = profileHeadings{Audience: []string{"Кому подойдет этот курс?"}, Requirements: []string{"Что нужно знать перед началом обучения?"}, Outcomes: []string{"Чему научитесь на курсе Python"}}
}
