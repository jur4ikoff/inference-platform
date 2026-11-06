#!/usr/bin/env python3
"""Render go test -json events as a standalone HTML test result report."""
import html
import json
import sys
from pathlib import Path

source, destination = map(Path, sys.argv[1:])
results = {}
output = {}
package_results = {}
package_output = {}
for line in source.read_text(encoding="utf-8").splitlines():
    event = json.loads(line)
    test = event.get("Test")
    if not test:
        package = event.get("Package", "")
        if event.get("Action") == "output":
            package_output.setdefault(package, []).append(event.get("Output", ""))
        if event.get("Action") in ("pass", "fail", "skip"):
            package_results[package] = event["Action"]
        continue
    key = (event.get("Package", ""), test)
    if event.get("Action") == "output":
        output.setdefault(key, []).append(event.get("Output", ""))
    if event.get("Action") in ("pass", "fail", "skip"):
        results[key] = event

counts = {status: sum(event["Action"] == status for event in results.values())
          for status in ("pass", "fail", "skip")}
rows = []
for (package, name), event in sorted(results.items()):
    status = event["Action"]
    detail = ""
    if status == "fail":
        detail = "<details><summary>Вывод</summary><pre>" + html.escape("".join(output.get((package, name), []))) + "</pre></details>"
    rows.append("<tr><td>" + html.escape(package) + "</td><td>" + html.escape(name) + "</td><td>" + status + "</td><td>" + str(event.get("Elapsed", 0)) + "</td><td>" + detail + "</td></tr>")
for package, result in sorted(package_results.items()):
    if result == "fail":
        detail = html.escape("".join(package_output.get(package, [])))
        rows.append("<tr><td>" + html.escape(package) + "</td><td>Сборка или запуск пакета</td><td>fail</td><td></td><td><pre>" + detail + "</pre></td></tr>")
package_failures = sum(result == "fail" for result in package_results.values())
page = """<!doctype html><html lang="ru"><meta charset="utf-8"><title>Результаты Go-тестов</title>
<style>body{{font:14px sans-serif;margin:2rem}}table{{border-collapse:collapse;width:100%}}td,th{{border:1px solid #ccc;padding:.4rem;text-align:left}}pre{{white-space:pre-wrap}}</style>
<h1>Результаты Go-тестов</h1><p>Успешно: {passed}; ошибок: {failed}; пропущено: {skipped}. Учитываются тесты и подзапуски; ошибок пакетов: {package_failures}.</p>
<table><thead><tr><th>Пакет</th><th>Тест</th><th>Результат</th><th>Секунды</th><th>Подробности</th></tr></thead><tbody>{rows}</tbody></table></html>""".format(
    passed=counts["pass"], failed=counts["fail"], skipped=counts["skip"], package_failures=package_failures, rows="\n".join(rows))
destination.write_text(page, encoding="utf-8")
