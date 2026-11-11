# Using eMudScan

## The window

<p align="center">
  <img src="images/main-window.png" width="860" alt="eMudScan’s main window after a scan: the online test option, the drives, the buttons, the log and the three lists">
</p>

From top to bottom:

- **eMudScan and its version.** The ⓘ icon opens the About box: author, online
  test, privacy, credits, licenses and disclaimer. **Language** (top right):
  Automatic (Windows), English or Français. Changing it resets the window; the
  choice is not saved.
- **Allow the online test**: checked by default. Hover over it to see what is
  sent; clear it to scan without any internet connection (see [Privacy](privacy.md)).
- **Drives to scan**: local drives are selected by default; DVD, removable and
  network drives are not. Nothing on an unselected drive is read or reported.
- **Scan** starts the scan (it never starts on its own). It becomes **Stop**
  during the scan, then **Scan again**. **Fix**, beside it, fixes the software
  that eMudScan can fix (see [Fixing software](#fixing-software)). **Copy report**
  and **Save report** are on the right.
- **Log**: one line per step. You can select and copy its text. The shield
  before its title is animated while the scan runs, then shows the worst status
  that the scan found. The right end of its top line shows what the scan is
  reading and the time, then how long the scan took.
- Three lists: **Windows certificate system**, **Browsers and internet apps**
  (browsers, messaging, email, sync, VPN, remote access…) and **Other software**
  (everything else). Drag the borders between them to resize them. The shield
  before the title of each list shows the worst status of its items (a grey one
  before the scan), and the right end of its top line counts them by status.
  **Hide OK items**, after the title, hides the OK items of a list, which its top
  line still counts (“5 OK (hidden)”).

## Statuses

In the lists, each status has a shield; the log and the report use a symbol.

| Shield | Symbol | Status | Meaning |
|:-:|:-:|---|---|
| <img src="images/status-ok.png" width="22" alt="green shield"> | ✅ | OK | No action needed |
| <img src="images/status-info.png" width="22" alt="blue shield"> | ℹ | Information | For your information; no action needed |
| <img src="images/status-warning.png" width="22" alt="yellow shield"> | ⚠ | Warning | May be affected: worth checking |
| <img src="images/status-update.png" width="22" alt="red shield"> | ❗ | Update needed | Accepts the compromised certificates until updated |
| <img src="images/status-problem.png" width="22" alt="dark red shield"> | ❌ | Critical | Windows itself still trusts the compromised certificates |

## The software lists

- **Software**: the software as you know it, with its version and, when it was
  installed with a package manager or from the Microsoft Store, how it was
  installed (`hugo 0.152.2 [Scoop]`). For a part of a program (when the
  **Component** column is filled in), the version is the program’s, as shown in
  **Apps & features** or by its package manager, so that all the parts of a
  program show the same version.
- **Component**: the part being checked (certifi, Electron, Java…); empty when
  the software itself is checked.
- **Software date**: when the publisher last updated the software
  ([how it is determined](checks.md#software-date)). When the software needs an
  update and is 12 months old or more, the date is shown in amber with its age:
  a newer version may exist, or the software may no longer be maintained.

  <img src="images/software-date-amber.png" width="180" alt="Software date: 07/07/2025 · 15 months, in amber">

- **Status**, **Outdated file**, **Software folder** and **Details**.

The same finding in several places (two copies of Node.js inside the same
program) is a single item: its **Outdated file** ends with *(+1)*, and its
detailed information lists every copy, then their explanation once, with the
commands for each copy. **Scan again** checks each copy; copies that no longer
match get their own items again.

Click a column header to sort, click a path to open it in File Explorer, and
hover over a cell to see its full content.

The search box above the lists (Ctrl+F) keeps the items whose **Software** or
**Component** column contains every word typed, in any order or case, including
versions and package managers (`node 22`, `certifi`, `scoop`). Once a scan
is finished, it has the focus: just type. The right
end of the top line of each list then counts the items found ("Search found 2
items: …"), and its shield shows their worst status. Esc clears it, as do
**Scan** and **Scan again**.

Right-click one or more items for: **Detailed information**, **Fix**,
**Open file location**, **Open software folder**, **Open homepage**,
**Search on Google**, **Scan again**, **Copy**, **Copy paths** and
**Select all**. **Fix** opens the **Fix software** window for the selected items
that can be fixed (see [Fixing software](#fixing-software)). **Scan again**
first reads the installed software again, so a program you have just updated
shows its new version.

With several items selected, each entry says how many things it acts on, and
counts once what the items share: for three items in two folders,
**Open 2 software folders**. **Search on Google** searches once per software,
and names what it searches for (the first three, then how many more), such as
**Search on Google: “cacert.pem”**. For a certificate bundle of a software that
eMudScan does not know, it searches for the software and the file’s name; for a
bundle that belongs to no software, for the file’s name alone. Several homepages
or searches open in one start of your default browser, as tabs. The entries that
open a window or a tab for each thing (**Open…** and **Search…**) ask first when
there are more than two.

## The detailed information window

Double-click an item (or press Enter) to see its software date and where that
date comes from, its folders, homepage and package, and the **recommended
action**. Commands are shown in black blocks, with a copy button, for you to run.
When eMudScan can run them for you, **Fix this software** does so, once you
confirm (see [Fixing software](#fixing-software)).

With several items selected, **Detailed information on 3 items…** (or Enter)
opens one window that shows them one at a time. The block in its top right
corner says which one is shown (**2 / 3**) and goes to the first, previous, next
or last item; it stays in place as the page scrolls. Alt+Home, Alt+←, Alt+→ and
Alt+End do the same. A link to another item opens that item on its own; closing
it brings you back.

A path in your user folder is shown with the variable that names that folder, as
File Explorer understands it: `%USERPROFILE%`, `%APPDATA%` or `%LOCALAPPDATA%`
(`%LOCALAPPDATA%\Programs\App\app.exe`). So are the paths of the report, the log
and the Fix window, and the commands name the folder as PowerShell does
(`"$env:LOCALAPPDATA\Programs\App\app.exe"`): they are shorter, and your user name
stays out of them. The links and the copied paths keep the full path.

<p align="center">
  <img src="images/details-window.png" width="560" alt="The detailed information window of Inkscape: its copy of certifi is outdated; update Inkscape, or replace the file with the commands shown, or with Fix this software">
</p>

Before changing a file, these commands back it up next to the original, with the
date and time in its name (`cacert.pem.emudscan-20261031-011500.bak`) and the
dates of the original, which a restore puts back too. As long as such a backup
exists, the detailed information window offers a command to restore it, and
**Restore the backup** to run it for you (see
[Restoring a backup](#restoring-a-backup)).

In C:\ProgramData, any user can create folders, and often add files to the
folders of others, so a program or a backup there may have been put there by
someone else. For files that need administrator rights, eMudScan therefore gives
the pip commands and the restore command only when the files and their folders
belong to administrators, SYSTEM, TrustedInstaller or you, and gives no pip
command for a Python whose folders other users can add files to: Python would
load what they put there.

For a browser that may not be protected, links open the test pages of
we-have-a-problem.com in that browser, so that you can check for yourself.
eMudScan starts only a browser registered in Windows or kept in a folder that
only administrators or you can change (Program Files, Windows, your user
folder): for another one, the detailed information gives the pages’ addresses to
open in it.

## Fixing software

eMudScan can run for you the commands that the detailed information gives to fix
a software, and no others:

- an update through the package manager that installed the software or its
  component: Scoop, WinGet, Chocolatey, the Python Install Manager, uv, pipx,
  pip or conda, or Deno’s own updater;
- an outdated certificate bundle replaced with Mozilla’s current one, which
  PowerShell downloads from curl.se and checks against the SHA-256 checksum that
  curl publishes;
- the compromised certificates added to a Java block list;
- the node.exe inside a program replaced with the latest release of its Node.js
  release line, which PowerShell downloads from nodejs.org and checks against
  the SHA-256 checksum that Node.js publishes, the OpenJS Foundation’s signature
  and its version; this fix is not ticked by default;
- the ssl files of a Python inside a program replaced with those of the fixed
  release of its line, which PowerShell downloads from python.org or
  python-build-standalone and checks against the SHA-256 checksum written in
  eMudScan, with tests before and after (see
  [Other software](checks.md#other-software)); this fix is not ticked by default.

The other commands of the detailed information, such as removing a setting, are
left for you to run; restoring a backup has its own button (see
[Restoring a backup](#restoring-a-backup)). Software that only its publisher,
Windows Update, the Microsoft Store or a network administrator can fix is not
listed: its detailed information says what to do.

**Fix**, beside **Scan**, is enabled once a scan has found software that
eMudScan can fix. It opens the **Fix software** window; **Fix** in the
right-click menu of the lists opens it for the selected items only. The window
has, from top to bottom:

- **Select All**, **Select None**, how many software are selected, and a
  reminder of what the list can do.
- The software that eMudScan can fix, with its vulnerable component, the fix,
  its location and, after the fixes, the result. They are all ticked, except
  those whose fix replaces a component inside a program (its `node.exe`, the ssl
  files of its Python), a heavier change: tick them yourself, or click
  **Select All**. A software that has such a fix and others has a row for each,
  so that the others stay ticked. A software found in several places has a row
  for each. Click a location to show it in File Explorer. Double-click a
  software to open its detailed information, where you can also fix it, or
  restore its backup: the list is updated when they close. Right-click a
  software for the menu of the main window: its detailed information, open its
  file, folder or homepage, search for it, scan it again, or copy its row
  (Ctrl+C) or its paths. Click a column header to sort the list by that column;
  click it again to reverse the order. Drag the bar below the list to make it
  taller or shorter, down to a single row.
- The PowerShell commands that will run for the ticked software (or for the
  software you click), grouped by software. If they run in a different order,
  the warning before they run explains it. The button in their top right corner
  copies them, with their comments, as in the detailed information window; after
  the fixes, it copies their results. You can also select part of the text and
  copy it (Ctrl+C, or right-click).
- **Back up the files to change**, **Fix the selected software**, **Stop** and
  **Close**.

<p align="center">
  <img src="images/fix-window.png" width="860" alt="The Fix software window: the software that eMudScan can fix, Inkscape ticked, and the PowerShell commands that replace its outdated certificate bundle">
</p>

**Fix this software**, in the detailed information window, opens the same window
for that software alone, without the list and without **Stop**, since a fix that
has started to change files always runs to its end; its button is
**Run the fix now**.

**Back up the files to change**, ticked by default, backs up each file before it
is replaced or changed (a certificate bundle, a Java block list, a node.exe, the
ssl files of a Python), next to the original, as the detailed information window
does (see above). It does not cover the updates made by package managers or pip,
so it is shown only when a fix to run backs up a file. The ssl files of a Python
are backed up in any case, so that the fix can put them back if a test fails; if
the box is cleared, these backups are deleted once the tests pass.

Nothing runs until you accept a warning, which says what the commands do: they
update software and replace files; package managers will not ask you anything,
as the commands confirm the updates and accept the license agreements for you;
and some changes cannot be undone. **Decline** is its default button. Close the
software to fix and save your work first.

<p align="center">
  <img src="images/fix-warning.png" width="520" alt="The warning shown before the fixes run: what the commands change, that some changes cannot be undone, and that eMudScan comes with no warranty; Decline is the default button">
</p>

Once you accept:

1. Updates (Scoop, WinGet, Chocolatey, Python Install Manager, uv, pipx and
   Deno’s updater) run first, followed by the other fixes for the software the
   updates did not fix. In each stage, commands that require administrator
   rights run last, together, so Windows asks for permission at most once per
   stage.
2. The commands that need administrator rights run in a Windows PowerShell for
   which Windows asks your permission, in a window that shows Microsoft as the
   verified publisher. eMudScan itself does not need them.
3. The console shows each command in the colors of PowerShell 7, with its
   result in the column on its left: “…” while it runs, then a green ✔ or a red
   ✘. Below come what it showed, its warnings in yellow, its errors in red, then
   its exit code when it failed, and a blank line after the commands of each
   software. **Stop** never stops a command in the middle: Windows PowerShell
   first ends the commands it was given (those of one software, or all those
   that need administrator rights), then a red line says “Fix interrupted!”, and
   the software not reached is left as it was. A failed connection stops the
   fixes in the same way, since the next ones would most likely fail too: a
   download that fails, before its fix has changed anything, or an update that
   fails when the servers of its package manager cannot be reached (see
   [Privacy](privacy.md#what-a-fix-connects-to)). A red line says why.
4. Each software is checked again: the console and the **Result** column say
   whether it is fixed, and the lists show its new status. In the console of the
   **Fix software** window, each result ends with a link to the detailed
   information of that software (**Detailed information on Inkscape…**). A fixed
   software loses its check box, so that it is not fixed twice: **Select All**
   leaves it out, and the count of selected software no longer includes it. A
   software still not fixed after all its fixes ran is unticked, so that they do
   not run again by mistake: tick it to run them again. When an update installed
   no newer version, its result says so (“still not fixed (no newer version
   installed)”). The summary of the certificate bundles is updated too: the
   files it lists are tested again.
5. A last line reminds you to restart any fixed software that was open: the fix
   takes effect the next time it starts.

<p align="center">
  <img src="images/fix-results.png" width="860" alt="The Fix software window after the fix of Inkscape: a green ✓ before each command, Inkscape checked again and fixed, with a link to its detailed information, and the link to the report saved in the Downloads folder">
</p>

The **Fix software** window then saves the report in your Downloads folder, and
gives its path, in a message and at the end of the console, as a link that shows
it in File Explorer.

<p align="center">
  <img src="images/report-saved.png" width="400" alt="The message that gives the path of the saved report, as a link that shows it in File Explorer">
</p>

To try the Fix function without changing anything, build the simulation: see
[Building and releasing](building.md#building-on-your-pc).

### Restoring a backup

Once a software has nothing left to fix, and as long as eMudScan’s backups of
its files are there, **Fix this software**, in its detailed information window,
becomes **Restore the backup** (or **Restore the backups**). Nothing changes
until you accept a warning, which says that the software will then trust the
compromised certificates again. eMudScan then moves each backup back in place of
its file, as the command of the detailed information does, and removes the files
that the fix added (the OpenSSL libraries added beside those of a Python). The
ssl files of a Python are restored only all together, when all the backups and
marks of that fix are there, since some of them alone would stop its `ssl`
module from loading; this also works after a fix that a crash interrupted during
the copy. A file in a folder that only administrators can change, such as
Program Files, is moved by that command, in a Windows PowerShell for which
Windows asks your permission: eMudScan itself does not need administrator
rights. The software is then checked again, and a message says whether its files
are back; if it needs fixing, its detailed information offers
**Fix this software** again. The report keeps what was done.

## The report

**Copy report** and **Save report** include every item, with the same
explanations as the detailed information windows and, after fixes, the commands
that ran and what they showed. Its top gives eMudScan’s version, the date, the
name of your PC and its Windows version. Before you share a report, remove what
you want to keep private, such as the name of your PC or your user name in the
paths. Apart from the fixes, saving a report is the only time eMudScan writes a
file.

**Save report** lets you choose the type of the file, in the list of types of
its dialog:

| Type | File | Use |
|---|---|---|
| Text | `.txt` | To read: it is what **Copy report** copies |
| JSON | `.json` | For other programs: every item with all its fields |
| CSV | `.csv` | For a spreadsheet: one row for each item |

The type you chose is offered first the next time, until you close eMudScan.
The content of a file is always what its name says: a name that you type with
one of these extensions gets its type.

The files are in UTF-8; the text and the CSV start with a byte order mark, for
Notepad and Excel. In the JSON and CSV files, the names of the fields and the
keywords are in English in both languages. The texts (summary, details, log)
are in the language of the window, and the paths are written as in the text
report.

### The JSON file

| Field | Content |
|---|---|
| `format`, `format_version` | `emudscan-report` and `1`. The version changes when a field is removed or changes meaning, not when a field is added |
| `tool` | eMudScan: `name`, `version`, `date` and `url`. A build that is not a release has `commit` instead of `date` |
| `created` | When the report was made, such as `2026-12-01T10:15:48+01:00` |
| `language` | `en` or `fr`: the language of the texts |
| `pc` | `name` and `windows`: the name of the PC and its Windows version |
| `scan` | `online_test`: the online test was allowed. `drives`: the drives scanned. `seconds`: how long the scan took. `complete`: `false` when the scan was stopped |
| `worst_status` | The worst status of the items |
| `counts` | The number of items of each status |
| `items` | The items, in the order of the text report |
| `fixes_run` | After fixes: the lines of the Fixes section of the text report |
| `log` | The lines of the log |

Each item has these fields; a field without a value is left out:

| Field | Content |
|---|---|
| `group` | Its list: `windows`, `internet` or `other` |
| `status` | `ok`, `information`, `warning`, `update_needed` or `critical` |
| `program`, `program_version` | The software. For a check of Windows: `Windows` |
| `component`, `component_version` | The part that is checked, when it is not the program itself. For a check of Windows: the name of the check |
| `summary` | The Details column of the lists |
| `details` | The explanation of the detailed information window, as text |
| `software_date` | The Software date column, such as `2026-03-02` |
| `file`, `folder` | The outdated file, or the file checked when the item is OK, and the software folder |
| `homepage` | The homepage of the software |
| `packages` | The packages of the software: `manager`, `id` and `page` |
| `copies` | When the same finding is in several places, the other places: `file` and `folder` |
| `fixes` | What **Fix** can run: `kind`, `manager` (for an update), `optional` (not ticked when the Fix window opens), `administrator` (needs administrator rights) and `commands` |

The kinds of fixes are `update` (with the package manager), `pip`, `conda`,
`tool`, `replace_bundle`, `java_block_list`, `replace_node` and
`replace_python_ssl`. An item whose check did not end has the status `unknown`,
which is also the worst status of a report without items.

### The CSV file

The CSV file has one row of column names, then one row for each item. The
values are separated by commas, whatever the regional settings of Windows. Its
columns:

`pc`, `windows`, `created`, `tool_version`, `group`, `status`, `program`,
`program_version`, `component`, `component_version`, `summary`,
`software_date`, `file`, `folder`, `copies`, `fix`

The values are those of the JSON fields. `copies` gives the other places of the
finding, and `fix` the kinds of its fixes, separated by ` | `. The explanations,
the commands and the log are only in the text and JSON files.

A value that starts with `=`, `+`, `-` or `@` gets an apostrophe before it, so
that a spreadsheet does not run it as a formula. If Excel shows each row in one
cell (its list separator is not the comma, as with French regional settings),
import the file with **Data** > **From Text/CSV**.

## Keyboard shortcuts

| Keys | Action |
|---|---|
| F5 | Scan, or scan again |
| Ctrl+S | Save the report |
| Ctrl+Shift+C | Copy the report |
| Ctrl+F | Search the lists |
| Ctrl+Q | Quit eMudScan |
| Enter | Show the detailed information of the selected items |
| Esc | Close the detailed information window |
| Alt+←, Alt+→ | In the detailed information of several items: previous, next item |
| Alt+Home, Alt+End | In the detailed information of several items: first, last item |
| Ctrl+C, Ctrl+A | In a list: copy the selected items, select all |

Hover over the **Scan**, **Copy report** and **Save report** buttons to see
their keyboard shortcuts.

## The About box

The ⓘ icon next to the version opens it: what eMudScan does, who made it, its
license, what fixes change and connect to, the online test and what it sends
(including your Windows edition and version), the reference data and the
disclaimer. **Licenses and notices** shows the full license texts.

## Command line

`eMudScan.exe` opens the window. It has two options:

- `--lang en`, `--lang fr`: the language for this session.
- `--offline`: start with **Allow the online test** cleared.

### Scanning without the window

`eMudScan-cli.exe` is a second program, for administrators who scan from a
script or a deployment tool. It is the same scanner, as a console program: it
works alone, without `eMudScan.exe`, and command prompts wait for it. It scans,
then exits. It asks nothing, never runs a fix, and needs no administrator
rights.

| Option | Effect |
|---|---|
| `--report FILE` | Write the report in FILE. Its extension gives the type: `.txt`, `.json` or `.csv` (text with another extension). Give the option several times to get several files. A file that exists is replaced |
| `--drives C:,D:` | The drives to scan. Without this option: the local drives, as the window ticks them. `--drives none`: no drive, only the checks that need none |
| `--online` | Run the online test. Without this option, the scan makes no internet connection |
| `--lang en`, `--lang fr` | The language of the log and the reports |
| `--help` | Show the options |

The log is written to the standard error output while the scan runs. Without
`--report`, the text report is then written to the standard output. A wrong
option stops the scan before it starts.

The exit code gives the worst status found:

| Exit code | Meaning |
|:-:|---|
| 0 | OK, or information only |
| 1 | Warning |
| 2 | Update needed |
| 3 | Critical |
| 10 | The scan could not run, or a report could not be written. The reason is on the standard error output |

In cmd:

```bat
eMudScan-cli.exe --report "%TEMP%\eMudScan-%COMPUTERNAME%.json"
echo %ERRORLEVEL%
```

In PowerShell:

```powershell
.\eMudScan-cli.exe --report "$env:TEMP\eMudScan.json"
$LASTEXITCODE
```

To keep a report, prefer `--report` to redirecting the standard output: the
file is then written by eMudScan itself, and no shell changes its characters.

## If your antivirus flags eMudScan.exe

Some antivirus programs flag new programs that are not code-signed, such as
eMudScan, even though they contain no malware: a false positive. To check your
copy of `eMudScan.exe`:

- Its SHA-256 must match the one on its
  [release page](https://github.com/3c0/eMudScan/releases). In PowerShell:
  `Get-FileHash eMudScan.exe`.
- The release page links to the results of an antivirus scan of
  `eMudScan.exe` made on the release date.
- With the [GitHub CLI](https://cli.github.com/), you can verify that GitHub
  built it from eMudScan’s source code:
  `gh attestation verify eMudScan.exe -R 3c0/eMudScan`.

Only your antivirus’s publisher can fix a false positive: report the file to
them, usually through a form on their website. eMudScan’s author cannot change
what an antivirus reports, so please do not report false positives in
eMudScan’s issues.

You can also build `eMudScan.exe` yourself from its source code: see
[Building and releasing](building.md).
