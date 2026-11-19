# Spec Delta

## ADDED Requirements

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
