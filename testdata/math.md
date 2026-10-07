# Formulas

A page to check how mdview shows math. Each section says what you should see.

## Inline

Euler's identity $e^{i\pi} + 1 = 0$ sits in the middle of a sentence.
Two formulas on one line: $a^2 + b^2 = c^2$ and $\sqrt{2} \approx 1.414$.

## Display

$$\int_0^1 x^2 \, dx = \frac{1}{3}$$

$$\sum_{k=1}^{n} k = \frac{n(n+1)}{2}$$

## Markdown characters inside a formula

None of these may turn into italics, bold or a lost character:

- underscores: $a_1 + a_2 + \dots + a_n$
- stars: $a * b * c$
- percent and braces: $50\% \text{ of } \{1, 2, 3\}$
- a matrix with `\\` line breaks: $$\begin{pmatrix} 1 & 0 \\ 0 & 1 \end{pmatrix}$$

## In other blocks

| Name      | Formula                    |
|-----------|----------------------------|
| Area      | $\pi r^2$                  |
| Quadratic | $x = \frac{-b \pm \sqrt{b^2 - 4ac}}{2a}$ |

1. A list item with $O(n \log n)$.
2. A nested formula in **bold text: $x^2$**.

> A quote with $\lim_{x \to 0} \frac{\sin x}{x} = 1$.

### A heading with $\alpha + \beta$

Cyrillic around a formula: площадь круга равна $\pi r^2$, а длина — $2\pi r$.

## Must stay as plain text

- In a code span: `$x^2$` and `\(x\)`.
- In a code block:

```latex
$$\int_0^1 f(x)\,dx$$
```

- An escaped dollar: this costs \$5.

## Raw HTML

The Math extension does not see these; MathJax still has to typeset them:

<div>\[ e^x = \sum_{n=0}^{\infty} \frac{x^n}{n!} \]</div>

<p>Inline in HTML: \( \nabla \cdot \vec{E} = \frac{\rho}{\varepsilon_0} \)</p>

## Edge cases

Two prices on one line must stay text: it costs $5 and $10.

A display formula over several lines must show as a formula. The Math
extension reads one line only, but MathJax finds the `$$` itself:

$$
\frac{a}{b}
$$
