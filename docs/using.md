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
  during the scan, then **Scan again**. **Copy report** and **Save report** are on
  the right.
- **Log**: one line per step. You can select and copy its text.
- Three lists: **Windows certificate system**, **Browsers and internet apps**
  (browsers, messaging, email, sync, VPN, remote access…) and **Other software**
  (everything else). Drag the borders between them to resize them.

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
  installed (`tesseract 5.5.1 [Scoop]`). For a part of a program (when the
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

The same finding in several places (two copies of Node.js inside LM Studio) is a
single item: its **Outdated file** ends with *(+1)*, and its details list every
copy. **Scan again** checks each copy; copies that no longer match get their own
items again.

Click a column header to sort, click a path to open it in File Explorer, and
hover over a cell to see its full content. **Hide OK items** filters a list.

The search box above the lists (Ctrl+F) keeps the items whose **Software** or
**Component** column contains every word typed, in any order or case, including
versions and package managers (`lm studio node`, `check`, `scoop`). Esc clears
it, as do **Scan** and **Scan again**.

Right-click one or more items for: **Details**, **Open file location**, **Open
software folder**, **Open homepage**, **Search on Google**, **Scan again**,
**Copy**, **Copy paths** and **Select all**. **Scan again** first reads the
installed software again, so a program you have just updated shows its new
version.

## The details window

Double-click an item (or press Enter) to see its software date and where that
date comes from, its folders, homepage and package, and the **recommended
action**. Commands are shown in black blocks, with a copy button. eMudScan never
runs them: you decide.

<p align="center">
  <img src="images/details-window.png" width="560" alt="The details window of Inkscape: its copy of certifi is outdated; update Inkscape, or replace the file with the commands shown">
</p>

Before changing a file, these commands back it up next to the original, with
the date and time in its name (`cacert.pem.emudscan-20261031-011500.bak`). As
long as such a backup exists, the details window offers a command to restore it.

For a browser that may not be protected, links open the advisory’s test pages in
that browser, so that you can check for yourself.

<p align="center">
  <img src="images/details-browser.png" width="560" alt="The details window of ungoogled-chromium: it may not receive the block list; links open the advisory’s five test pages in it">
</p>

## The report

**Copy report** and **Save report** (UTF-8 text) include every item, with the
same explanations as the details windows. Saving a report is the only time
eMudScan writes a file.

## Keyboard shortcuts

| Keys | Action |
|---|---|
| F5 | Scan, or scan again |
| Ctrl+S | Save the report |
| Ctrl+Shift+C | Copy the report |
| Ctrl+F | Search the lists |
| Ctrl+Q | Quit eMudScan |
| Enter | Show the details of the selected items |
| Esc | Close the details window |
| Ctrl+C, Ctrl+A | In a list: copy the selected items, select all |

Hover over the **Scan**, **Copy report** and **Save report** buttons to see
their keyboard shortcuts.

## The About box

The ⓘ icon next to the version opens it: what eMudScan does, who made it, its
license, the online test and what it sends, the reference data and the
disclaimer. **Licenses and notices** shows the full license texts.

<p align="center">
  <img src="images/about-box.png" width="440" alt="eMudScan’s About box">
</p>

## Command line

- `--en`, `--fr`: the language for this session.
- `--offline`: start with **Allow the online test** cleared.
