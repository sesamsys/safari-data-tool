# Requirements Document

## Introduction

This feature adds an interactive terminal user interface (TUI) mode to the safari-data-tool macOS command-line tool. Today, running the tool with no arguments produces the same non-interactive listing as the `list` subcommand. This feature changes the bare-invocation default so that running the tool with no arguments and no subcommand launches an interactive, full-screen TUI that lets the operator browse the largest Safari website-data folders, navigate them with the arrow keys, and delete a selected folder (with confirmation) without re-running the tool.

The TUI is implemented in pure Go using only the standard library. Raw terminal mode is established through termios control, and all input parsing (arrow keys, command keys) and all rendering (inverted selection, redraws, legend) are done with hand-written ANSI escape sequences. The feature is macOS-only. The existing `list` and `delete` subcommands retain their current non-interactive behavior and are not changed by this feature.

The TUI reuses existing building blocks: `scanner.Scan` for enumeration and access classification, `selection.TopN` for size-ranked selection, `ui.FormatLine` for row rendering, `ui.Confirm`'s detail set (Domain, Hash, Size) for the deletion prompt, `ui.AccessMessage` for access guidance, and `deleter` for the actual removal.

## Glossary

- **Tool**: The safari-data-tool command-line program as a whole.
- **TUI**: The interactive terminal user interface introduced by this feature.
- **Bare_Invocation**: Running the Tool with no subcommand and no positional/flag arguments after the program name.
- **Folder_List**: The ordered set of scanned folders presented in the TUI, limited to the fifteen largest folders by recursive size, sorted largest-first.
- **Listing_Limit**: The fixed maximum number of folders shown in the Folder_List, equal to 15.
- **Selected_Item**: The single folder in the Folder_List that currently has focus and is eligible for deletion.
- **Selection_Index**: The zero-based position of the Selected_Item within the Folder_List.
- **Inverted_Rendering**: Rendering a row with a light background and dark text (reverse video) to indicate the Selected_Item.
- **Key_Legend**: A short, always-visible help line listing the available TUI commands and their keys for the current screen. Its content is context-sensitive: the Folder_List and All_Clear_State show the navigation/delete/quit keys, while the Confirmation_Prompt shows the confirm/cancel keys.
- **Access_Status**: The `model.AccessStatus` value returned by `scanner.Scan` (`StatusOK`, `StatusNotFound`, or `StatusAccessBlocked`).
- **Scanned_Folder**: A `model.ScannedFolder` record with `Hash`, `AbsPath`, `SizeKB`, `Domain`, and `Hosts` fields.
- **Confirmation_Prompt**: The in-TUI prompt that shows the Selected_Item's Domain, Hash, and Size and requires an affirmative or negative keypress before deletion.
- **Raw_Mode**: The terminal input mode in which keypresses are delivered to the Tool unbuffered and without echo, established via termios control.
- **Initial_Scan**: The first scan the TUI performs at launch, before any folder has been rendered or deleted in the current session.
- **Session_Has_Loaded**: A per-session flag that is true once the TUI has, at least once in the current session, received an Access_Status of `StatusOK` and built a Folder_List containing at least one folder.
- **All_Clear_State**: The TUI state entered when a re-scan after a successful deletion returns an empty result (Access_Status of `StatusAccessBlocked` caused by zero remaining folders) while Session_Has_Loaded is true, indicating the operator has interactively removed every Safari website-data folder rather than lacking Full Disk Access.
- **All_Clear_Message**: The empty-state text shown in the All_Clear_State, such as "No Safari website-data folders remain.", displayed in place of the `ui.AccessMessage` guidance.

## Requirements

### Requirement 1: Interactive activation on bare invocation

**User Story:** As an operator, I want running the tool with no arguments to open an interactive browser, so that I can explore and clean up Safari data without memorizing subcommands.

#### Acceptance Criteria

1. WHEN the Tool receives a Bare_Invocation, THE Tool SHALL launch the TUI.
2. WHEN the Tool receives a first argument of `list`, THE Tool SHALL run the existing non-interactive listing flow and SHALL NOT launch the TUI.
3. WHEN the Tool receives a first argument of `delete`, THE Tool SHALL run the existing non-interactive deletion flow and SHALL NOT launch the TUI.
4. WHERE the TUI is active, THE Tool SHALL use only the Go standard library for terminal control, input parsing, and rendering.

### Requirement 2: Scan and populate the folder list

**User Story:** As an operator, I want the TUI to show the largest Safari website-data folders ranked by size, so that I can focus on the folders worth reclaiming.

#### Acceptance Criteria

1. WHEN the TUI launches, THE TUI SHALL scan the Safari WebsiteData base path using the existing scanner before rendering the Folder_List.
2. WHEN the scan returns an Access_Status of `StatusOK`, THE TUI SHALL build the Folder_List from the largest folders by recursive size.
3. THE TUI SHALL limit the Folder_List to the Listing_Limit of 15 folders.
4. THE TUI SHALL order the Folder_List by descending recursive size, with the largest folder first.
5. WHEN the TUI renders a folder row, THE TUI SHALL format the row using the existing `ui.FormatLine` rendering.
6. WHILE the TUI is active, THE TUI SHALL display the Key_Legend at all times, with its content reflecting the commands available in the current screen.
7. WHEN a scan returns an Access_Status of `StatusOK` AND the resulting Folder_List contains at least one folder, THE TUI SHALL set Session_Has_Loaded to true.

### Requirement 3: Arrow-key navigation with wrap-around

**User Story:** As an operator, I want to move the selection up and down with the arrow keys, so that I can pick a folder without typing identifiers.

#### Acceptance Criteria

1. WHEN the TUI finishes populating the Folder_List, THE TUI SHALL set the Selection_Index to the topmost folder.
2. WHEN the operator presses the Down arrow key AND the Selected_Item is not the last folder, THE TUI SHALL move the Selection_Index to the next folder.
3. WHEN the operator presses the Up arrow key AND the Selected_Item is not the first folder, THE TUI SHALL move the Selection_Index to the previous folder.
4. WHEN the operator presses the Down arrow key AND the Selected_Item is the last folder, THE TUI SHALL move the Selection_Index to the first folder.
5. WHEN the operator presses the Up arrow key AND the Selected_Item is the first folder, THE TUI SHALL move the Selection_Index to the last folder.
6. WHEN the Selection_Index changes, THE TUI SHALL redraw the Folder_List so the Selected_Item reflects the new position.

### Requirement 4: Selected-item highlighting

**User Story:** As an operator, I want the current selection to stand out visually, so that I always know which folder a delete action will target.

#### Acceptance Criteria

1. WHILE the Folder_List is displayed, THE TUI SHALL render the Selected_Item with Inverted_Rendering.
2. WHILE the Folder_List is displayed, THE TUI SHALL render every folder other than the Selected_Item without Inverted_Rendering.

### Requirement 5: Deletion with confirmation

**User Story:** As an operator, I want to delete the selected folder after confirming the details, so that I can reclaim space without accidentally removing the wrong folder.

#### Acceptance Criteria

1. WHEN the operator presses `d` or `D`, THE TUI SHALL display the Confirmation_Prompt for the Selected_Item.
2. WHEN the TUI displays the Confirmation_Prompt, THE TUI SHALL show the Selected_Item's Domain, Hash, and Size.
3. WHEN the Confirmation_Prompt is displayed AND the operator presses `y` or `Y`, THE TUI SHALL delete the Selected_Item's folder.
4. WHEN the Confirmation_Prompt is displayed AND the operator presses `n` or `N`, THE TUI SHALL cancel the deletion and return to the Folder_List with the Selection_Index unchanged.
5. IF deletion of the Selected_Item's folder fails, THEN THE TUI SHALL display the failure to the operator and return to the Folder_List.
6. WHEN the Confirmation_Prompt is displayed, THE TUI SHALL display the Key_Legend showing the confirm and cancel keys above the prompt line.
7. WHEN the Confirmation_Prompt is displayed, THE TUI SHALL render the prompt as the final line of the frame so the operator's input appears directly after it.

### Requirement 6: Re-scan after deletion

**User Story:** As an operator, I want the list to refresh itself after a deletion, so that I always see the current state of my Safari data.

#### Acceptance Criteria

1. WHEN a deletion completes successfully, THE TUI SHALL re-scan the Safari WebsiteData base path and rebuild the Folder_List from the current scan results.
2. WHEN the TUI rebuilds the Folder_List after a deletion, THE TUI SHALL apply the Listing_Limit of 15 and the descending-size ordering.
3. WHEN the rebuilt Folder_List contains at least one folder, THE TUI SHALL set the Selection_Index to a valid position within the rebuilt Folder_List.
4. IF a re-scan after a successful deletion returns an empty result AND Session_Has_Loaded is true, THEN THE TUI SHALL enter the All_Clear_State as defined in Requirement 9 instead of rendering the `ui.AccessMessage` guidance.

### Requirement 7: Quit the TUI

**User Story:** As an operator, I want a single key to exit the interactive mode, so that I can leave cleanly whenever I am done.

#### Acceptance Criteria

1. WHEN the operator presses `q` or `Q`, THE TUI SHALL exit interactive mode and return control to the operator's shell.
2. WHEN the TUI exits, THE TUI SHALL restore the terminal to the input mode that was in effect before Raw_Mode was established.

### Requirement 8: Access-status handling

**User Story:** As an operator, I want clear guidance when Safari data cannot be read, so that I understand what to fix instead of seeing an empty screen.

#### Acceptance Criteria

1. WHEN the Initial_Scan returns an Access_Status of `StatusAccessBlocked`, THE TUI SHALL display the guidance returned by `ui.AccessMessage` for that status and SHALL NOT render the Folder_List.
2. WHEN the Initial_Scan returns an Access_Status of `StatusNotFound`, THE TUI SHALL display the guidance returned by `ui.AccessMessage` for that status and SHALL NOT render the Folder_List.
3. WHEN the Initial_Scan returns a non-OK Access_Status, THE Tool SHALL terminate with a non-zero exit code.
4. WHERE a re-scan after a successful deletion returns an empty result AND Session_Has_Loaded is true, THE Tool SHALL apply the All_Clear_State of Requirement 9 and SHALL NOT terminate with a non-zero exit code.

### Requirement 9: All-clear empty state after clearing all folders

**User Story:** As an operator, I want the TUI to tell me when I have removed every Safari website-data folder, so that I see a clear "all clean" message instead of misleading Full Disk Access guidance.

#### Acceptance Criteria

1. WHEN a scan returns an Access_Status of `StatusOK` AND the resulting Folder_List contains at least one folder, THE TUI SHALL record that Session_Has_Loaded is true for the remainder of the session.
2. WHEN a re-scan after a successful deletion returns an empty result AND Session_Has_Loaded is true, THE TUI SHALL enter the All_Clear_State.
3. WHILE the TUI is in the All_Clear_State, THE TUI SHALL display the All_Clear_Message and SHALL NOT display the `ui.AccessMessage` guidance.
4. WHILE the TUI is in the All_Clear_State, THE TUI SHALL display the Key_Legend.
5. WHILE the TUI is in the All_Clear_State, THE TUI SHALL remain interactive and SHALL accept keypresses from the operator.
6. WHILE the TUI is in the All_Clear_State AND the operator presses `q` or `Q`, THE TUI SHALL exit interactive mode and return control to the operator's shell.
7. WHEN the TUI exits from the All_Clear_State, THE Tool SHALL terminate with a zero exit code.
