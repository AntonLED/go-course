#!/usr/bin/env bash
# Публикует курс на GitHub и включает GitHub Pages.
#
#   ./publish.sh              # репозиторий go-course
#   ./publish.sh my-go-book   # своё имя репозитория
#
# Нужны git и GitHub CLI (brew install gh; gh auth login).
# Повторный запуск просто отправляет новые изменения — сайт пересоберётся сам.
set -euo pipefail
cd "$(dirname "$0")"

REPO="${1:-go-course}"

command -v git >/dev/null || { echo "Нужен git"; exit 1; }
command -v gh  >/dev/null || { echo "Нужен GitHub CLI: brew install gh && gh auth login"; exit 1; }
gh auth status >/dev/null 2>&1 || { echo "Сначала войдите: gh auth login"; exit 1; }

OWNER="$(gh api user --jq .login)"

if [ ! -d .git ]; then
  git init -q -b main
fi
git add -A
git diff --cached --quiet || git commit -q -m "Курс по Go: обновление"

if ! git remote get-url origin >/dev/null 2>&1; then
  if gh repo view "$OWNER/$REPO" >/dev/null 2>&1; then
    git remote add origin "https://github.com/$OWNER/$REPO.git"
  else
    gh repo create "$OWNER/$REPO" --public --source=. --remote=origin --description "Курс подготовки по Go: конспекты, задачи с тестами, квизы"
  fi
fi
# Включить Pages с публикацией через GitHub Actions до первого push, чтобы первая же сборка опубликовалась.
if ! gh api "repos/$OWNER/$REPO/pages" >/dev/null 2>&1; then
  gh api -X POST "repos/$OWNER/$REPO/pages" -f build_type=workflow >/dev/null \
    || echo "Не удалось включить Pages автоматически: Settings → Pages → Source: GitHub Actions"
fi

git push -u origin main

if [[ "$REPO" == *.github.io ]]; then URL="https://$REPO/"; else URL="https://$OWNER.github.io/$REPO/"; fi
echo
echo "Готово. Сборка идёт здесь: https://github.com/$OWNER/$REPO/actions"
echo "Через пару минут сайт будет доступен: $URL"
