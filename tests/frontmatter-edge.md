---
title: "Edge cases: quotes, & ampersands, <angles>"
empty_value:
single_tag: [solo]
empty_list: []
number: 42
boolean: true
date: 2026-09-01
nested:
  level2:
    level3: deeply nested value
refs:
  - name: first ref
    page: 1
  - name: second ref
    page: 2
tags: [alpha, beta, gamma, delta, epsilon, zeta, eta, theta, iota, kappa, lambda, mu]
long_value: "A deliberately long single value that should wrap inside its cell rather than forcing the table to overflow horizontally past the reading column."
---

# Edge cases body

The properties table above should show every row from the frontmatter.

| Sortable | Doc table |
| --- | --- |
| b | 2 |
| a | 1 |

Clicking the header of *this* table should sort it; the properties table must not
be sortable.
