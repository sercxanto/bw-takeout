# bw-profile Specification

## Purpose

Defines the CLI data directory of a run: a fresh directory that is separate from the user's own CLI profile, is removed when the run ends, and stays visible when a run could not remove it.

## Requirements

### Requirement: Fresh profile for every run
Each run SHALL create a new, empty profile directory under the profile location of this project. A directory that already exists SHALL NOT be used as a profile, and the user's own CLI profile SHALL NOT be used.

#### Scenario: Two runs in the same second
- **WHEN** two runs create their profile within the same second
- **THEN** each run has its own profile directory with a different name

#### Scenario: Chosen name already exists
- **WHEN** the chosen profile name already exists
- **THEN** another name is chosen
- **AND** the existing directory and its content are unchanged

#### Scenario: Profile location is missing
- **WHEN** the profile location does not exist yet
- **THEN** it is created
- **AND** the profile is created inside it

### Requirement: Profile is readable by the owner only
On systems with POSIX permissions, the profile directory and the directories inside it SHALL be created so that only the owner can read or enter them.

#### Scenario: Permissions on Linux and macOS
- **WHEN** a profile is created on Linux or macOS
- **THEN** the profile directory and its subdirectories have mode 0700

### Requirement: Separate data and working directories
A profile SHALL provide one directory for the CLI data and a different, empty directory as the working directory of `bw` processes.

#### Scenario: New profile
- **WHEN** a profile is created
- **THEN** it has a data directory and a working directory
- **AND** both are empty

### Requirement: Teardown removes the profile
Teardown SHALL attempt the logout and then remove the profile directory with everything in it. The directory SHALL be removed also when the logout fails. A failed logout and a failed removal SHALL each be reported, and a failed removal SHALL name the directory.

#### Scenario: Teardown after a successful run
- **WHEN** teardown runs after a login
- **THEN** the logout is attempted
- **AND** the profile directory no longer exists

#### Scenario: Logout fails
- **WHEN** the logout fails during teardown
- **THEN** the profile directory no longer exists
- **AND** the failed logout is reported

#### Scenario: Removal fails
- **WHEN** the profile directory cannot be removed
- **THEN** the failure is reported with the path of the directory

#### Scenario: Teardown without login
- **WHEN** teardown runs for a profile in which no login succeeded
- **THEN** the profile directory no longer exists

### Requirement: Leftover profiles are reported and never removed
A run SHALL be able to list the profile directories of other runs in the profile location, each with its name and age. It SHALL NOT modify or remove them, because a leftover of an ended run cannot be told from the profile of a running one.

#### Scenario: Leftover from a killed run
- **WHEN** the profile location holds a profile directory of a run that was killed
- **THEN** the directory is listed with its name and age
- **AND** the directory and its content are unchanged

#### Scenario: Own profile
- **WHEN** a run lists leftovers while its own profile exists
- **THEN** its own profile is not listed

#### Scenario: Unrelated entries in the profile location
- **WHEN** the profile location holds a file or a directory whose name does not have the form of a profile name
- **THEN** the entry is not listed
- **AND** it is unchanged
