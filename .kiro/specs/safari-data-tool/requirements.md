# Requirements Document

## Introduction

Safari Data Tool is a standalone, pure-Go command-line utility for macOS that inspects and manages Safari's WebKit WebsiteData storage. It replaces an existing `safari-data` zsh function. The tool lists the largest per-domain data folders under the Safari container, resolves each hashed folder to a human-readable domain by parsing the plaintext `origin` files WebKit stores, reports accurate recursive folder sizes, and allows an operator to permanently delete a specific domain's data by domain name or hash with a confirmation safeguard. The tool is distributed as a public GitHub repository with build and usage documentation.

## Glossary

- **Safari_Data_Tool**: The command-line application defined by this specification.
- **WebsiteData_Base_Path**: The directory `$HOME/Library/Containers/com.apple.Safari/Data/Library/WebKit/WebsiteData/Default`, under which Safari stores per-domain website data.
- **Hashed_Folder**: A directory located directly under the WebsiteData_Base_Path whose name is a hash string and whose contents correspond to a single registrable domain.
- **Origin_File**: A file literally named `origin` located inside a Hashed_Folder or one of its subfolders, containing length-prefixed records that map the folder to a scheme and host.
- **Origin_Record**: One entry within an Origin_File, encoded as a 4-byte little-endian unsigned length value `N`, followed by one flag byte (`0x01`), followed by `N` bytes of UTF-8 text.
- **Primary_Domain**: The host value of the first scheme/host record pair found in an Origin_File, used as the resolved domain for a Hashed_Folder.
- **Resolved_Domain**: The Primary_Domain determined for a Hashed_Folder, or the placeholder `Unknown Domain` when no Origin_File can be parsed.
- **Folder_Size**: The total recursive disk usage of a Hashed_Folder, computed equivalently to `du -sk` (sum of file sizes within the folder tree).
- **Listing_Count**: The number of largest Hashed_Folders to display, defaulting to 15 and configurable by the operator.
- **Force_Flag**: A command-line option (`--force` / `--yes`) that skips the deletion confirmation prompt.
- **Full_Disk_Access**: The macOS TCC permission that the host terminal application requires in order to read the TCC-protected Safari container contents.
- **Operator**: The person running the Safari_Data_Tool from a terminal.

## Requirements

### Requirement 1: List largest folders with configurable count

**User Story:** As an operator, I want to see the largest Safari website-data folders, so that I can identify which domains consume the most disk space.

#### Acceptance Criteria

1. WHEN the Operator runs the listing command without specifying a count, THE Safari_Data_Tool SHALL display the 15 largest Hashed_Folders ordered by descending Folder_Size.
2. WHERE the Operator provides a Listing_Count value, THE Safari_Data_Tool SHALL display that number of largest Hashed_Folders ordered by descending Folder_Size.
3. WHEN THE Safari_Data_Tool displays a Hashed_Folder in the listing, THE Safari_Data_Tool SHALL display the Folder_Size in human-readable units, the Resolved_Domain, and the Hashed_Folder name.
4. IF the requested Listing_Count exceeds the number of available Hashed_Folders, THEN THE Safari_Data_Tool SHALL display all available Hashed_Folders.
5. IF the Operator provides a Listing_Count that is not a positive integer, THEN THE Safari_Data_Tool SHALL report an input error and SHALL exit without listing.

### Requirement 2: Resolve domains via origin files

**User Story:** As an operator, I want each hashed folder resolved to its real domain, so that I can recognize which site each folder belongs to.

#### Acceptance Criteria

1. WHEN THE Safari_Data_Tool resolves a Hashed_Folder, THE Safari_Data_Tool SHALL locate an Origin_File within the Hashed_Folder or its subfolders.
2. WHEN THE Safari_Data_Tool parses an Origin_File, THE Safari_Data_Tool SHALL read each Origin_Record as a 4-byte little-endian length value, followed by one flag byte, followed by that many UTF-8 text bytes.
3. WHEN THE Safari_Data_Tool parses an Origin_File containing at least one scheme/host record pair, THE Safari_Data_Tool SHALL set the Resolved_Domain to the host value of the first record pair.
4. IF no Origin_File can be located or parsed for a Hashed_Folder, THEN THE Safari_Data_Tool SHALL set the Resolved_Domain to `Unknown Domain`.

### Requirement 3: Compute folder sizes

**User Story:** As an operator, I want accurate folder sizes, so that the reported disk usage matches what the system reports.

#### Acceptance Criteria

1. WHEN THE Safari_Data_Tool computes a Folder_Size, THE Safari_Data_Tool SHALL sum the sizes of all files within the Hashed_Folder directory tree recursively.
2. WHEN THE Safari_Data_Tool displays a Folder_Size, THE Safari_Data_Tool SHALL format the value in human-readable units.

### Requirement 4: Selective deletion by domain and hash with confirmation

**User Story:** As an operator, I want to delete a specific domain's data by domain or hash, so that I can reclaim space for a chosen site.

#### Acceptance Criteria

1. WHEN the Operator requests deletion by domain name, THE Safari_Data_Tool SHALL select the Hashed_Folder whose Resolved_Domain matches the provided domain name.
2. WHEN the Operator requests deletion by Hashed_Folder name, THE Safari_Data_Tool SHALL select the Hashed_Folder whose name matches the provided value.
3. IF the deletion target matches no Hashed_Folder, THEN THE Safari_Data_Tool SHALL report that no matching folder was found and SHALL exit without deleting.
4. WHILE no Force_Flag is provided, THE Safari_Data_Tool SHALL display the target Resolved_Domain, Hashed_Folder name, and Folder_Size and SHALL require explicit Operator confirmation before deleting.
5. IF the Operator declines the confirmation prompt, THEN THE Safari_Data_Tool SHALL exit without deleting.
6. WHEN the Operator confirms deletion or provides the Force_Flag, THE Safari_Data_Tool SHALL recursively delete the selected Hashed_Folder.
7. WHERE the Force_Flag is provided, THE Safari_Data_Tool SHALL delete the selected Hashed_Folder without displaying the confirmation prompt.

### Requirement 5: Path-safety guarantees for deletion

**User Story:** As an operator, I want deletion restricted to Safari's data directory, so that no files outside that location are ever removed.

#### Acceptance Criteria

1. WHEN THE Safari_Data_Tool resolves a deletion target path, THE Safari_Data_Tool SHALL verify that the resolved absolute path is contained within the WebsiteData_Base_Path.
2. IF a resolved deletion target path is not contained within the WebsiteData_Base_Path, THEN THE Safari_Data_Tool SHALL refuse the deletion and SHALL report a path-safety error.
3. WHEN THE Safari_Data_Tool validates a deletion target, THE Safari_Data_Tool SHALL confirm the target is a direct child Hashed_Folder of the WebsiteData_Base_Path before deleting.

### Requirement 6: Full Disk Access detection and messaging

**User Story:** As an operator, I want a clear message when permissions are missing, so that I understand why no data appears and how to fix it.

#### Acceptance Criteria

1. IF the WebsiteData_Base_Path exists but THE Safari_Data_Tool reads zero Hashed_Folders from it, THEN THE Safari_Data_Tool SHALL report that Full_Disk_Access is likely required.
2. WHEN THE Safari_Data_Tool reports a suspected Full_Disk_Access restriction, THE Safari_Data_Tool SHALL instruct the Operator to grant Full Disk Access to the terminal application in System Settings > Privacy & Security.
3. IF the WebsiteData_Base_Path does not exist, THEN THE Safari_Data_Tool SHALL report that the Safari website-data directory was not found.

### Requirement 7: macOS platform scoping

**User Story:** As an operator, I want the tool scoped to macOS, so that it behaves predictably on the only supported platform.

#### Acceptance Criteria

1. WHEN THE Safari_Data_Tool determines the WebsiteData_Base_Path, THE Safari_Data_Tool SHALL derive the path from the current Operator home directory using the macOS Safari container layout.
2. THE Safari_Data_Tool SHALL be built as a pure-Go binary compiled without CGO.

### Requirement 8: Repository and distribution

**User Story:** As an operator, I want build and usage documentation in a public repository, so that I can build, install, and run the tool safely.

#### Acceptance Criteria

1. THE Safari_Data_Tool repository SHALL include a README that documents build and installation instructions.
2. THE Safari_Data_Tool repository SHALL include a README that documents command usage for listing and deletion.
3. THE Safari_Data_Tool repository SHALL include a README that documents the Full_Disk_Access requirement.
4. THE Safari_Data_Tool repository SHALL include a README that states that deletion is permanent and irreversible.
