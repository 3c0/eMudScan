package main

import (
	"strings"
	"testing"
)

// The pieces of a command give it back, each with the kind PowerShell 7 colors it as.
func TestPSTokens(t *testing.T) {
	kinds := map[psKind]string{psPlain: "plain", psCommand: "command", psParameter: "parameter", psVariable: "variable", psString: "string",
		psKeyword: "keyword", psOperator: "operator", psType: "type", psNumber: "number", psComment: "comment"}
	for _, c := range []struct {
		cmd  string
		want []string // kind:text of the pieces that are not plain
	}{
		{`[Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12`,
			[]string{"type:[Net.ServicePointManager]", "operator:=", "type:[Net.ServicePointManager]", "operator:-bor", "type:[Net.SecurityProtocolType]"}},
		{`$cacert = $null; $cacert = (Invoke-WebRequest -UseBasicParsing -Uri https://curl.se/ca/cacert.pem).RawContentStream.ToArray()`,
			[]string{"variable:$cacert", "operator:=", "variable:$null", "operator:;", "variable:$cacert", "operator:=", "command:Invoke-WebRequest", "parameter:-UseBasicParsing", "parameter:-Uri"}},
		{`if (-not $ok) { Write-Warning 'The download failed: it''s incomplete.' }`,
			[]string{"keyword:if", "operator:-not", "variable:$ok", "command:Write-Warning", "string:'The download failed: it''s incomplete.'"}},
		{`& "$env:LOCALAPPDATA\Demo\python\python.exe" -m pip install --upgrade pip certifi`,
			[]string{"operator:&", `string:"`, "variable:$env:LOCALAPPDATA", `string:\Demo\python\python.exe"`, "parameter:-m", "parameter:--upgrade"}},
		{`foreach ($n in $pyFiles) { Copy-Item -LiteralPath (Join-Path $pyDir "$n.emudscan-$pyStamp.bak") -Force }`,
			[]string{"keyword:foreach", "variable:$n", "keyword:in", "variable:$pyFiles", "command:Copy-Item", "parameter:-LiteralPath", "command:Join-Path", "variable:$pyDir",
				`string:"`, "variable:$n", "string:.emudscan-", "variable:$pyStamp", `string:.bak"`, "parameter:-Force"}},
		{`$pyOK = $LASTEXITCODE -eq 0`, []string{"variable:$pyOK", "operator:=", "variable:$LASTEXITCODE", "operator:-eq", "number:0"}},
		{`$env:UV_PYTHON_INSTALL_DIR = 'C:\uv'; uv python upgrade 3.12`, []string{"variable:$env:UV_PYTHON_INSTALL_DIR", "operator:=", "string:'C:\\uv'", "operator:;", "command:uv"}},
		{`if (uv python find --system --managed-python '>=3.12.16,<3.13' 2>$null) { uv python uninstall cpython-3.12.9-windows-x86_64-none } else { Write-Warning 'kept' }`,
			[]string{"keyword:if", "command:uv", "parameter:--system", "parameter:--managed-python", "string:'>=3.12.16,<3.13'", "number:2", "operator:>", "variable:$null",
				"command:uv", "keyword:else", "command:Write-Warning", "string:'kept'"}},
		{`scoop update uv`, []string{"command:scoop"}},
		{`$a = [bool](New-Item -ItemType File -Force -Path $p) # a comment`, []string{"variable:$a", "operator:=", "type:[bool]", "command:New-Item", "parameter:-ItemType",
			"parameter:-Force", "parameter:-Path", "variable:$p", "comment:# a comment"}},
		{`$x = $list[0]; $y = $x.Count`, []string{"variable:$x", "operator:=", "variable:$list", "number:0", "operator:;", "variable:$y", "operator:=", "variable:$x"}},
	} {
		toks := psTokens(c.cmd)
		var back strings.Builder
		var got []string
		for _, tk := range toks {
			back.WriteString(tk.text)
			if tk.kind != psPlain {
				got = append(got, kinds[tk.kind]+":"+strings.TrimSpace(tk.text))
			}
		}
		if back.String() != c.cmd {
			t.Errorf("not given back: %q", back.String())
		}
		if strings.Join(got, " | ") != strings.Join(c.want, " | ") {
			t.Errorf("%s\ngot  %s\nwant %s", c.cmd, strings.Join(got, " | "), strings.Join(c.want, " | "))
		}
	}
}

// Every command that eMudScan can show is given back whole.
func TestPSTokensWhole(t *testing.T) {
	for _, cmds := range [][]string{pySSLFix(`C:\Apps\X\python\python.exe`, pySSLPlan{pbsSSL["3.11"], `C:\Apps\X\python\DLLs`, pySSLShapes[2].replace, pySSLShapes[2].added}).Plain} {
		for _, c := range cmds {
			var b strings.Builder
			for _, tk := range psTokens(c) {
				b.WriteString(tk.text)
			}
			if b.String() != c {
				t.Errorf("not given back: %q", c)
			}
		}
	}
}
