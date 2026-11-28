---
title: A pending replacement file can leave the old import draft submittable
date: 2026-11-28
category: ui-bugs
module: Manual Import
problem_type: ui_bug
component: frontend
symptoms:
  - "Preview remains available for the previous draft while a replacement file is being read."
root_cause: async_timing
resolution_type: code_fix
severity: medium
tags: [manual-import, async-state, file-upload, drafts]
---

# A pending replacement file can leave the old import draft submittable

## Problem

Selecting a replacement JSON file starts an asynchronous read. A review found that the user could preview and submit the previous draft before that read finished. Starting the import then invalidated the pending read, so the replacement the user intended to import was ignored.

## Root Cause

The input revision establishes which read may apply its result. The previous draft was still valid and available for submission while the read was pending.

## Solution

The wizard tracks whether the latest file read is pending and disables Preview and submission eligibility during that interval. A newer edit or reset supersedes the read. If the latest read fails, the wizard clears the pending state, displays an error, and preserves the previous draft for recovery. [Manual Import wizard](../../../components/manual-import-view.tsx#L285)

## Prevention

When replacing editable input asynchronously, test both the pending interval and the order in which reads resolve. Start with a valid draft, select a slow replacement file, and try to preview the old draft before the read completes. Then paste newer JSON and allow the read to finish. The browser check verified that Preview was disabled during the read and the newer paste survived its completion.
