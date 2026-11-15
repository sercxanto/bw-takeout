# Spec Delta

## Purpose

Defines how the unmodified `bw` program is started and how its results are read, so that no secret leaves the process boundary, no command waits on a prompt, and no failure is taken for a success.

## ADDED Requirements

### Requirement: Child environment from an allowlist
Every `bw` process SHALL receive an environment built from a fixed allowlist of variables and from values the invocation sets itself. A variable of the calling environment that is not on the allowlist SHALL NOT reach the process. This includes every variable that changes the behavior, output or exit code of `bw` or of its runtime.

#### Scenario: Exit code override in the calling environment
- **WHEN** the calling environment sets `BW_CLEANEXIT` and a command fails
- **THEN** the `bw` process does not receive the variable
- **AND** the command is reported as failed

#### Scenario: Runtime options in the calling environment
- **WHEN** the calling environment sets `NODE_OPTIONS` and `NODE_TLS_REJECT_UNAUTHORIZED`
- **THEN** the `bw` process receives neither variable

#### Scenario: Certificate authority file configured
- **WHEN** an additional certificate authority file is configured for the invocation
- **THEN** the `bw` process receives it in `NODE_EXTRA_CA_CERTS`

#### Scenario: Certificate authority file only in the calling environment
- **WHEN** the calling environment sets `NODE_EXTRA_CA_CERTS` and no file is configured
- **THEN** the `bw` process does not receive the variable

### Requirement: Every process uses the run's data directory
Every `bw` process, including the version query, SHALL be started with the CLI data directory of the run. Before a login is started, the invocation SHALL confirm that `bw` uses this directory, and SHALL stop with an error when it does not.

#### Scenario: Version query
- **WHEN** the version of `bw` is queried
- **THEN** the process is started with the data directory of the run

#### Scenario: bw uses another data directory
- **WHEN** `bw` runs and leaves the given data directory empty
- **THEN** an error is reported
- **AND** no login is started

### Requirement: No interaction outside the login
Every command except the login SHALL run with interaction disabled, with an empty standard input and with a time limit. A command that exceeds its limit SHALL be stopped and reported as timed out.

#### Scenario: Locked vault
- **WHEN** a vault command runs while the vault is locked and the status afterwards reports locked
- **THEN** the command ends with a failure of the class locked
- **AND** it does not wait for input

#### Scenario: Command does not end
- **WHEN** a command produces no result within its time limit
- **THEN** the process is stopped
- **AND** the failure has the class timeout

#### Scenario: Run is cancelled
- **WHEN** the caller cancels while a command is running
- **THEN** the process is stopped
- **AND** the failure has the class cancelled

### Requirement: Session key stays in the child environment
The session key SHALL be passed to `bw` only in the environment of a child process, and only between a successful login and the logout. It SHALL NOT appear in an argument, a file, an error or any output of this project.

#### Scenario: Command after login
- **WHEN** a vault command runs after a successful login
- **THEN** the process receives the session key in its environment
- **AND** no argument of the process contains it

#### Scenario: Command before login
- **WHEN** the version, the status or the server configuration is queried before a login
- **THEN** the process receives no session key

#### Scenario: Command after logout
- **WHEN** the status is queried after the logout
- **THEN** the process receives no session key

#### Scenario: Failure with a session key in use
- **WHEN** a command fails after a successful login
- **THEN** the reported failure does not contain the session key

### Requirement: Interactive login needs a terminal
The login SHALL be started only when standard input and standard error are a terminal. It SHALL connect both to the `bw` process and read standard output through a pipe. Without a terminal the login SHALL be refused before any process is started.

#### Scenario: Login without a terminal
- **WHEN** a login is requested and standard input is not a terminal
- **THEN** the login is refused
- **AND** no `bw` process is started

#### Scenario: Login with a terminal
- **WHEN** a login is requested with a terminal
- **THEN** the `bw` process reads from and prompts on the terminal
- **AND** its standard output is not shown on the terminal

### Requirement: Session key form decides the login result
A login SHALL count as successful only when the process exits with code 0 and its standard output is exactly one session key: base64 text that decodes to 64 bytes, optionally followed by one line ending. Any other output SHALL be reported as a failed login.

#### Scenario: Exit code 0 with empty output
- **WHEN** the login process exits with code 0 and writes nothing to standard output
- **THEN** the login is reported as failed

#### Scenario: Valid session key
- **WHEN** the login process exits with code 0 and writes 88 base64 characters that decode to 64 bytes
- **THEN** the login is reported as successful

#### Scenario: Session key followed by a line ending
- **WHEN** the output is a valid session key followed by one line ending
- **THEN** the login is reported as successful

#### Scenario: Other text next to the key
- **WHEN** the output holds a valid session key and additional text
- **THEN** the login is reported as failed

#### Scenario: Non-zero exit code with a key
- **WHEN** the login process exits with a non-zero code and writes a valid session key
- **THEN** the login is reported as failed

### Requirement: Interrupted login leaves a usable terminal
When a login ends by an interrupt or a failure, the invocation SHALL wait for the `bw` process to end, restore the terminal state it found before the login, and report the login as failed, so that the caller can tear down.

#### Scenario: Interrupt at the password prompt
- **WHEN** the user interrupts the login at the password prompt
- **THEN** the terminal state from before the login is restored
- **AND** the login is reported as failed after the process has ended

### Requirement: Output stays in memory
Standard output of a `bw` process SHALL be held in memory only and SHALL NOT be written to a file by this project. Every process SHALL run in an empty working directory of the run. A file that appears there SHALL make the command fail. Output larger than the configured limit SHALL stop the command.

#### Scenario: Export
- **WHEN** an export runs
- **THEN** the document is returned from memory
- **AND** no file exists in the working directory afterwards

#### Scenario: bw writes a file
- **WHEN** a command leaves a file in the working directory
- **THEN** the command is reported as failed

#### Scenario: Output exceeds the limit
- **WHEN** a command writes more to standard output than the limit allows
- **THEN** the process is stopped
- **AND** the command is reported as failed

### Requirement: Results are read from the output
A command SHALL count as successful only when the exit code is 0 and the output has the form expected for the command. The status SHALL be read from the status document and never from the exit code. A missing value in the status document SHALL be reported as missing.

#### Scenario: Status of a profile without login
- **WHEN** the status is queried in a profile without login and the process exits with code 0
- **THEN** the status is reported as unauthenticated

#### Scenario: Status without a sync time
- **WHEN** the status document holds no time of the last sync
- **THEN** the time of the last sync is reported as missing

#### Scenario: Status document does not parse
- **WHEN** the status process exits with code 0 and its output is not the expected document
- **THEN** the command is reported as failed

#### Scenario: Unknown status value
- **WHEN** the status document names a state that is not known
- **THEN** the command is reported as failed

### Requirement: Failures carry a class and no CLI text
A failed command SHALL be reported with the operation, a class from a fixed list and the exit code. Text that `bw` wrote to standard output or standard error SHALL NOT be copied into a failure, a result or any output of this project. The classes not logged in and locked SHALL be taken from the status document. Text of `bw` SHALL select a class only for premium required on an attachment download. No class SHALL make a result more favorable than a general failure.

#### Scenario: Failed download with a name in the error text
- **WHEN** an attachment download fails and the error text of `bw` contains a sentinel value
- **THEN** the reported failure contains the item ID and the attachment ID
- **AND** it does not contain the sentinel value

#### Scenario: Not logged in
- **WHEN** a vault command fails in a profile without login and the status afterwards reports unauthenticated
- **THEN** the failure has the class not logged in

#### Scenario: Not-logged-in text with an unlocked vault
- **WHEN** a vault command fails with the text "You are not logged in." and the status afterwards reports unlocked
- **THEN** the failure has the general class for a failed command

#### Scenario: Status query after a failure fails
- **WHEN** a vault command fails and the status query that follows fails as well
- **THEN** the failure has the general class for a failed command

#### Scenario: Premium required
- **WHEN** an attachment download ends with the message that premium status is required
- **THEN** the failure has the class premium required

#### Scenario: Unrecognized error text
- **WHEN** a command exits with code 1 and an error text that is not recognized, and the status afterwards reports unlocked
- **THEN** the failure has the general class for a failed command

### Requirement: Decryption error lines are counted
The invocation SHALL count the decryption error lines that a command writes to standard error and SHALL report the count together with the result. The count SHALL NOT turn a successful command into a failed one.

#### Scenario: Export with decryption errors
- **WHEN** an export exits with code 0 and writes three decryption error lines to standard error
- **THEN** the export is reported as successful
- **AND** the reported count is three

#### Scenario: Export without error lines
- **WHEN** an export exits with code 0 and writes nothing to standard error
- **THEN** the reported count is zero

### Requirement: Identity of bw is reported and never judged
The invocation SHALL report the version that `bw` prints and the SHA-256 of the executable file. It SHALL NOT refuse, warn or change any result because of the version or the hash.

#### Scenario: Version unknown to this project
- **WHEN** `bw` prints a version this project was never tested with
- **THEN** every command runs as for any other version
- **AND** no message about the version is produced

#### Scenario: Version output is empty
- **WHEN** the version query exits with code 0 and prints nothing
- **THEN** the query is reported as failed

### Requirement: Attachments are requested by ID
An attachment SHALL be requested by item ID and attachment ID only. Both IDs SHALL have the form the archive accepts before they are used as arguments. An ID of another form SHALL be refused before any process is started.

#### Scenario: Download by ID pair
- **WHEN** an attachment is requested with a valid item ID and attachment ID
- **THEN** the process is asked for exactly this pair
- **AND** the content is returned from memory

#### Scenario: ID that starts with a dash
- **WHEN** an attachment is requested with an ID that starts with a dash
- **THEN** the request is refused
- **AND** no `bw` process is started

### Requirement: Server address is checked before use
The server address SHALL be an HTTPS URL with a host and without user information. Any other address SHALL be refused before any process is started.

#### Scenario: Address without HTTPS
- **WHEN** a server address with the scheme `http` is configured
- **THEN** the address is refused
- **AND** no `bw` process is started

#### Scenario: HTTPS address
- **WHEN** an HTTPS address is configured in a profile without login
- **THEN** `bw` is given this address
- **AND** the status afterwards reports it as the server
