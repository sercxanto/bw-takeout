# Spec Delta

## ADDED Requirements

### Requirement: Organizations are chosen by flag
The backup command SHALL accept the organization flag any number of times, each naming one organization ID to back up. An ID that does not have the UUID form, or an ID given twice, SHALL be a usage error, reported before any `bw` process starts. A run without the flag SHALL back up the personal vault only.

#### Scenario: Two organizations
- **WHEN** the backup command runs with the organization flag twice and two different IDs of UUID form
- **THEN** both organizations are chosen in the order given

#### Scenario: Organization ID not a UUID
- **WHEN** the organization flag is given the value `../x`
- **THEN** the command exits with the usage status
- **AND** no `bw` process was started and no profile was created

#### Scenario: Same organization twice
- **WHEN** the organization flag is given the same ID twice
- **THEN** the command exits with the usage status before any `bw` process starts

#### Scenario: No organization flag
- **WHEN** the backup command runs without the organization flag
- **THEN** no organization is chosen
- **AND** the organization list is not read

## MODIFIED Requirements

### Requirement: The target is untouched until the snapshot is complete
The run SHALL NOT create anything in the target before the forced sync, the freshness check, the check of the chosen organizations, the four documents and the documents of the chosen organizations have succeeded. A run that ends before that point SHALL leave the target as it was.

#### Scenario: Stale snapshot
- **WHEN** the snapshot is not fresh
- **THEN** the run ends with the status for no generation
- **AND** the target holds exactly the entries it held before the run

#### Scenario: Chosen organization without the role
- **WHEN** the account has the role user in a chosen organization
- **THEN** the run ends with the status for no generation
- **AND** the target holds exactly the entries it held before the run

#### Scenario: Organization export fails
- **WHEN** the encrypted export of a chosen organization fails
- **THEN** the run ends with the status for no generation
- **AND** the target holds exactly the entries it held before the run

### Requirement: Plaintext stays in memory
The run SHALL NOT write any document, attachment or other vault content to a file. It SHALL overwrite the captured documents, the four documents and those of the chosen organizations, in memory once the capture is parsed, and on every path that ends the run after the snapshot.

#### Scenario: No temporary files
- **WHEN** a run publishes a generation with attachments while the temporary directory of the process is an empty directory
- **THEN** that directory is still empty after the run

#### Scenario: No temporary files with an organization
- **WHEN** a run with a chosen organization publishes a generation while the temporary directory of the process is an empty directory
- **THEN** that directory is still empty after the run

### Requirement: Report shows the setup and the leftovers
Before the login, the report SHALL name the `bw` executable with its version and SHA-256, the server, the target, the number of recipients and the ID of each chosen organization. It SHALL list leftover profiles of other runs and leftover partial generations in the target, each with its path and age, and SHALL NOT modify or remove them.

#### Scenario: Leftovers present
- **WHEN** the profile location holds a profile of another run and the target holds a partial generation
- **THEN** the report lists both with their paths and ages
- **AND** both are unchanged after the run

#### Scenario: Chosen organizations
- **WHEN** the run has two chosen organizations
- **THEN** the setup lines name both organization IDs in the order given

### Requirement: Report shows how the run ended
The report SHALL name the published generation by its path, or state that none was published. It SHALL name a failure that ended the run by phase, `bw` operation, class, the IDs of a failed download, and the organization ID of a failed organization export or check, with the reason of the check. It SHALL state the result of the logout and of the profile removal, and the profile path when the removal failed. When the status names no server address, it SHALL name the address the run was given.

#### Scenario: Profile not removed
- **WHEN** the profile directory cannot be removed
- **THEN** the report names its path

#### Scenario: Status without server address
- **WHEN** the status reports no server address
- **THEN** the report names the server address from the flags

#### Scenario: Organization check failed
- **WHEN** the run ends because the account is not a member of a chosen organization
- **THEN** the report names the phase, the organization ID and that the account is not a member

#### Scenario: Organization export failed
- **WHEN** the run ends because the plaintext export of a chosen organization failed
- **THEN** the report names the phase, the operation, its class and the organization ID

### Requirement: Report holds no vault content and no bw text
The report SHALL NOT contain item names, user names, passwords, notes, field values, key material, attachment file names, organization names, the session key or text that `bw` wrote. Characters that are not printable SHALL be replaced in every value it prints. The prompts of `bw` during the login reach the terminal directly and are not part of the report.

#### Scenario: Sentinels in the vault and in bw output
- **WHEN** the vault holds sentinel values in names, notes, passwords and file names, and every `bw` command other than the login writes a sentinel to standard error
- **THEN** no sentinel appears in the report on standard output or in the messages of the run on standard error

#### Scenario: Sentinel in an organization name
- **WHEN** a chosen organization has a sentinel value as its name and the run publishes a generation
- **THEN** no sentinel appears in the report on standard output or in the messages of the run on standard error

#### Scenario: Control characters in the account address
- **WHEN** the status reports an account address with a terminal escape sequence
- **THEN** the report prints it with the control characters replaced
