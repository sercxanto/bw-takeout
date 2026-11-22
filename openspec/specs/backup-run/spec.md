# backup-run Specification

## Purpose

Runs the interactive backup of one account from the command line: it checks the inputs, logs in at the terminal, collects and verifies a capture, publishes a generation by its verdict, tears down on every path and reports the outcome without vault content.

## Requirements

### Requirement: Backup inputs are flags
The backup command SHALL require a server address, a target directory and a recipients file, and SHALL accept an optional `bw` executable path and an optional certificate authority file. A missing required flag, an unknown flag, an extra argument or a server address that is not an HTTPS URL with a host and without user information SHALL be a usage error, reported before any `bw` process starts.

#### Scenario: Server flag missing
- **WHEN** the backup command runs without a server address
- **THEN** it exits with the usage status
- **AND** no `bw` process was started and no profile was created

#### Scenario: Server address not HTTPS
- **WHEN** the server address is `http://vault.example.org`
- **THEN** it exits with the usage status before any `bw` process starts

### Requirement: Recipients come from a file
The recipients file SHALL hold one age recipient per line. Empty lines and lines starting with `#` SHALL be skipped. A file without any recipient, or with a line that is neither a recipient nor skipped, SHALL be refused. The refusal SHALL name the line number and never the content of the line.

#### Scenario: Two recipients with a comment
- **WHEN** the recipients file holds a comment line, an empty line and two recipients
- **THEN** the archive is encrypted to both recipients

#### Scenario: Invalid line
- **WHEN** the third line of the recipients file is not a recipient
- **THEN** the run refuses the file, names line 3 and does not print the line

### Requirement: Inputs are checked before the first bw process
Before it starts any `bw` process, the run SHALL check that the target is an existing directory, that the recipients file yields at least one recipient, that the `bw` executable exists, given or found on the program search path, and that a given certificate authority file is a readable file. A failed check SHALL end the run with no generation, without starting `bw` and without creating a profile.

#### Scenario: Target missing
- **WHEN** the target directory does not exist
- **THEN** the run ends with the status for no generation
- **AND** no `bw` process was started and no profile was created

#### Scenario: bw not found
- **WHEN** no `bw` path is given and the program search path holds no `bw`
- **THEN** the run ends with the status for no generation before any profile is created

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

### Requirement: Logout after every login attempt
Once the login was started, teardown SHALL call the logout, also when the login failed, because `bw` can be logged in although its session key was refused. Teardown SHALL remove the profile on every path of the run, including failures, a failed verdict and interrupts.

#### Scenario: Login refused
- **WHEN** the login ends with an output that is not a session key
- **THEN** the logout is called
- **AND** the profile directory no longer exists
- **AND** the run ends with the status for no generation

#### Scenario: Failure before the login
- **WHEN** the version query fails
- **THEN** no login and no logout are started
- **AND** the profile directory no longer exists

### Requirement: Logout is bounded
The logout SHALL end after at most 10 seconds. A logout that has not ended by then SHALL be stopped and counted as not successful. The profile SHALL then be removed as on any other path.

#### Scenario: Logout hangs
- **WHEN** the logout does not end within 10 seconds
- **THEN** it is stopped and reported as not successful
- **AND** the profile directory no longer exists

### Requirement: Interrupts end the run and keep teardown
The first interrupt SHALL end the run. A generation that is not committed yet SHALL be aborted, so that nothing of it stays in the target. The run SHALL print a message on standard error that the logout follows within the limit and that another interrupt skips it. It SHALL then tear down.

#### Scenario: Interrupt during the attachments
- **WHEN** an interrupt arrives while an attachment is downloaded
- **THEN** no generation and no partial directory of this run are in the target
- **AND** the message about the logout is printed on standard error
- **AND** the logout is called and the profile directory no longer exists
- **AND** the run ends with the status for no generation

#### Scenario: Interrupt before the target is touched
- **WHEN** an interrupt arrives during the snapshot
- **THEN** the target holds exactly the entries it held before the run

### Requirement: An interrupt during teardown ends the logout
An interrupt during teardown SHALL stop the logout at once and SHALL count it as not successful. Removing the profile SHALL NOT be interrupted. An interrupt that arrives after the first one but before the logout starts SHALL make the logout end without starting a `bw` process.

#### Scenario: Second interrupt during the logout
- **WHEN** a second interrupt arrives while the logout runs
- **THEN** the logout is stopped
- **AND** the profile directory no longer exists
- **AND** the report states that the logout was skipped

### Requirement: The run decides on interrupts itself
The run SHALL record that it was interrupted and SHALL report an interrupted run as interrupted, whatever class the failure of the last `bw` command has. On Unix, the interrupt, quit, terminate and hangup signals SHALL count as interrupts.

#### Scenario: bw ends on its own after the interrupt
- **WHEN** an interrupt arrives and the running `bw` command ends with a non-zero exit code before the run stops it
- **THEN** the report states that the run was interrupted

#### Scenario: Terminate signal
- **WHEN** the run receives the terminate signal on Unix
- **THEN** it ends the run and tears down as for an interrupt

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

### Requirement: Report shows progress per attachment
The report SHALL show each attachment download in order with its position and its result. It SHALL name the item ID, the attachment ID and the error class for a failed download only, and SHALL NOT name attachment file names.

#### Scenario: One download fails
- **WHEN** the second of three attachments fails after its last attempt
- **THEN** the report shows three results in order
- **AND** only the second names its item ID, attachment ID and error class

### Requirement: Report shows the verdict in full
The report SHALL show the verdict, the scope statement, the standing limits, every finding and every warning, one per line with severity, reason and IDs. It SHALL do so for every verdict, also when no generation is published.

#### Scenario: Failed verdict
- **WHEN** the verification result is failed with two findings
- **THEN** the report shows the verdict, the scope, the limits and both findings
- **AND** states that no generation was published

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

### Requirement: Exit status of a backup
The exit status SHALL be 0 for a published generation with a verified capture and a complete teardown, 3 for a published generation with an unverified capture and a complete teardown, and 4 for a published generation whose logout was not successful or whose profile was not removed. It SHALL be 1 when no generation is published, and 2 for a usage error.

#### Scenario: Verified backup
- **WHEN** the capture is verified, the generation is published, the logout succeeds and the profile is removed
- **THEN** the exit status is 0

#### Scenario: Unverified backup
- **WHEN** the capture is unverified and the generation is published with the unverified marker
- **THEN** the exit status is 3

#### Scenario: Published generation, logout failed
- **WHEN** the generation is published and the logout fails
- **THEN** the exit status is 4
- **AND** the report names the published generation

#### Scenario: Failed verdict
- **WHEN** the verification result is failed
- **THEN** the exit status is 1

#### Scenario: No generation and profile not removed
- **WHEN** the snapshot fails and the profile cannot be removed
- **THEN** the exit status is 1

### Requirement: Core dumps are limited before the first bw process
On Linux and macOS, before it starts any `bw` process and before it creates a profile, the run SHALL set the soft and the hard core file size limit of its process to zero and SHALL confirm both by reading them back. Every `bw` process the run starts SHALL inherit both limits.

#### Scenario: bw inherits the limit
- **WHEN** a run on Linux or macOS starts a `bw` process
- **THEN** that process sees a soft and a hard core file size limit of zero

#### Scenario: Limit set before the first bw process
- **WHEN** a run on Linux or macOS fails a later check, such as a missing target
- **THEN** the soft and the hard core file size limit of the process are zero when the run ends

### Requirement: The run process is not dumpable on Linux
On Linux, before it starts any `bw` process and before it creates a profile, the run SHALL mark its own process as not dumpable and SHALL confirm the mark by reading it back. The mark SHALL NOT be required of the `bw` processes, because starting a program resets it.

#### Scenario: Run process not dumpable
- **WHEN** a run on Linux has passed the checks
- **THEN** its process is marked as not dumpable

### Requirement: A run that cannot limit crash dumps does not start bw
On Linux and macOS, a run whose core file size limit or, on Linux, whose not-dumpable mark cannot be set or does not read back as set SHALL end in the checks phase with no generation, without creating a profile and without starting `bw`. The failure SHALL name which of the two failed and SHALL hold no other detail.

#### Scenario: Core limit cannot be set
- **WHEN** setting the core file size limit fails
- **THEN** the run ends with the status for no generation
- **AND** the report names the checks phase and the core limit
- **AND** no `bw` process was started and no profile was created

#### Scenario: Core limit does not read back as zero
- **WHEN** the core file size limit reads back as a value other than zero after it was set
- **THEN** the run ends with the status for no generation before any profile is created

#### Scenario: Not-dumpable mark cannot be set on Linux
- **WHEN** marking the process as not dumpable fails on Linux
- **THEN** the run ends with the status for no generation
- **AND** the report names the checks phase and the dumpable mark
- **AND** no `bw` process was started and no profile was created

### Requirement: Windows runs state that crash dumps are not limited
On Windows, the run SHALL NOT change any crash dump setting, of its process or of the system, and SHALL continue. The report SHALL state that crash dumps are not limited on this platform.

#### Scenario: Run on Windows
- **WHEN** a run on Windows has passed the checks
- **THEN** the report states that crash dumps are not limited on this platform
- **AND** the run goes on to create its profile

### Requirement: Report names the crash dump protection
With the setup lines, the report SHALL name the crash dump protection of the run: on Linux the core limit and the not-dumpable mark, on macOS the core limit, and on Windows that crash dumps are not limited.

#### Scenario: Run on Linux
- **WHEN** a run on Linux has passed the checks
- **THEN** the setup lines state that the core limit is zero and that the process is not dumpable

#### Scenario: Run on macOS
- **WHEN** a run on macOS has passed the checks
- **THEN** the setup lines state that the core limit is zero

### Requirement: A core file left by bw ends the run
A file that a `bw` process leaves in its working directory, such as a core file that the system writes there after a crash, SHALL end the run with no generation, and teardown SHALL remove the profile together with that file.

#### Scenario: bw leaves a core file
- **WHEN** a `bw` command during the snapshot leaves a file named `core` in its working directory
- **THEN** the run ends with the status for no generation
- **AND** the target holds exactly the entries it held before the run
- **AND** the profile directory, with the file, no longer exists after the run

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
