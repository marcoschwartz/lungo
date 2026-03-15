package lungo

import (
    "testing"
    "strings"
)

func TestDollarBeforeExpr(t *testing.T) {
    tests := []struct {
        name   string
        input  string
        want   string
        notWant string
    }{
        {
            "dollar before expression",
            `function X() { return (<span>${plan.price === 0 ? "Free" : "$" + plan.price}</span>); }`,
            `"$"`,  // $ should be a separate text child
            "",
        },
        {
            "dollar in text",
            `function X() { return (<p>Cost: $100</p>); }`,
            `"Cost: $100"`,
            "",
        },
        {
            "no dollar",
            `function X() { return (<p>{name}</p>); }`,
            `name`,
            `"$"`,
        },
        {
            "dollar amount with expression",
            `function X() { return (<p>Total: ${amount}</p>); }`,
            `"Total: $"`,
            "",
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            out := TranspileJSX(tt.input)
            if tt.want != "" && !strings.Contains(out, tt.want) {
                t.Errorf("expected %q in output, got:\n%s", tt.want, out)
            }
            if tt.notWant != "" && strings.Contains(out, tt.notWant) {
                t.Errorf("did not expect %q in output, got:\n%s", tt.notWant, out)
            }
        })
    }
}
