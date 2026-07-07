---
description: Adversarial review of a feature spec or plan
argument-hint: [request]
---

Следуйте файлу ".speckeep/templates/prompts/challenge.md".

Команда: `/spk.challenge [request]`

Аргументы пользователя:
{{arguments}}

Требования:
- сначала прочитайте project.constitution_file (по умолчанию CONSTITUTION.md), если это требуется prompt-файлом
- Если в фазе нужна конституция, сначала загрузите `.speckeep/constitution.summary.md`, если файл существует; только при его отсутствии переходите к `project.constitution_file`.
- Trace placement: никогда не ставьте `@sk-task`/`@sk-test` на уровень `package`, `import` или file-header comment; размещайте маркер непосредственно над owning function/method/test/type declaration (или над явным behavioral block header, если в языке нет таких объявлений).
- используйте только минимально нужный контекст репозитория
- Строго сохраните точную финальную строку из prompt-файла: `Готово к: ...` или `Вернуться к: ...` без перефразирования и без пропуска.


