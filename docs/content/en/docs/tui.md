---
title: Using the TUI
weight: 25
---

Every screen shows its keys in the help line at the bottom. These work the same everywhere.

## Moving around

| Key | Does |
|---|---|
| `↑` `↓` (or `k` `j`) | Move through a menu; the chosen row is highlighted using your terminal's own colours |
| `Enter` | Choose / go to the next question |
| `Esc` | Go back one screen (cancel a wizard) |
| `Ctrl+C` | Quit |

## Wizards: go back, then review

Questions are asked one at a time, and you are never stuck with an answer:

- **`Shift+Tab`** (or `Ctrl+B`, or `↑` on a text field) goes back to the previous question. Your earlier
  answers are kept, and a question that no longer applies after you change a choice is dropped.
- After the last question a **review** shows, in plain words, what is about to change, for example
  "Visitors are passed on to the app at 127.0.0.1:8080" and "Served over HTTPS with the Let's Encrypt
  certificate for example.com (42d left)". It never shows Nginx configuration. `Enter` applies, `Shift+Tab`
  goes back, `Esc` cancels. Nothing is written before you apply, and as always the result is tested with
  `nginx -t` and put back if the test fails.

Wizards with no summary of their own review the questions and your answers instead (secrets are hidden).

## Searching a list

Wherever there is a list (hosts, certificates, access lists, IP lists, upstream groups, streams, rate limits,
error pages, country lists, history, and any choice of six or more options in a wizard), press **`Ctrl+F`**.
The box above the list says what it searches, for example *domain, alias or backend* for hosts.

- It is **fuzzy**: `shpex` finds `shop.example.com`. A contiguous match always ranks above a scattered one,
  and the best matches come first. A single letter only matches as plain text.
- Several words must **all** match: `shop org`.
- Matching characters are underlined and reversed, so you can see why a row matched.
- While the box is open every key is typed into it (so `q` or `r` do not trigger anything). `↑` `↓` move
  through the results, `Enter` opens one, and `Esc` closes the search.
- Rows such as **+ Create …** stay in the list while you search.

## Notices

Important information is shown in a coloured block with a bar on its left: red for certificates that are
expired or have 14 days or less left (see [SSL certificates]({{< relref "ssl" >}})), blue for information such
as the review of a change or a new release (see [Install]({{< relref "install" >}})).
