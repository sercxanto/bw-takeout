# generation-publishing Specification

## Purpose

Defines how a backup generation appears in the target directory so that older backups are never touched, concurrent runs cannot collide, and an incomplete or unverified generation is recognizable from its name.

## Requirements

### Requirement: Every generation gets a new name
Each generation SHALL be created as a new directory in the target whose name contains the creation time in UTC and a random part. A name that already exists SHALL NOT be reused.

#### Scenario: Two generations in the same second
- **WHEN** two generations are published within the same second
- **THEN** the target holds two directories with different names

#### Scenario: Name collision
- **WHEN** the chosen name already exists in the target
- **THEN** another name is chosen
- **AND** the existing directory and its content are unchanged

### Requirement: Existing content is never modified
Publishing SHALL NOT modify, replace or remove any file or directory that existed in the target before the run, whatever the outcome of the run.

#### Scenario: Successful run next to older generations
- **WHEN** a generation is published into a target that holds older generations and unrelated files
- **THEN** every earlier file is byte-identical afterwards

#### Scenario: Failing run next to older generations
- **WHEN** a run fails at any step into a target that holds older generations
- **THEN** every earlier file is byte-identical afterwards

### Requirement: Concurrent runs do not collide
Several runs that publish into the same target at the same time SHALL each produce their own generation or fail without affecting the others.

#### Scenario: Concurrent runs with identical time and random source
- **WHEN** eight runs publish into the same target at the same time, with the same clock value and the same first random value
- **THEN** eight complete generations with different names exist
- **AND** each one passes offline verification

### Requirement: Incomplete generations are marked
A generation SHALL carry the marker `.partial` in its directory name from its creation until its data is completely written and flushed to storage. A generation without this marker SHALL be complete. A generation whose archive was not finished SHALL NOT be published, whatever its verdict.

#### Scenario: Write error in the middle
- **WHEN** writing fails after part of the archive was written
- **THEN** no directory without the marker was created by this run

#### Scenario: Unfinished archive
- **WHEN** a verified capture is written and finishing the archive fails before its manifest is written, while every byte written so far reached the file
- **THEN** no directory of this run remains in the target

#### Scenario: Process killed while writing
- **WHEN** the writing process is killed in the middle of the archive
- **THEN** the target holds a directory with the marker for this run and none without it

### Requirement: The verdict decides publication
A generation whose verification result is verified SHALL be published under its plain name. One whose result is unverified SHALL be published with the marker `.unverified` in its directory name. One whose result is failed SHALL NOT be published, and its partial directory SHALL be removed.

#### Scenario: Verified capture
- **WHEN** a generation is written from a capture with the verdict verified
- **THEN** its directory name has neither marker

#### Scenario: Unverified capture
- **WHEN** a generation is written from a capture with the verdict unverified
- **THEN** its directory name ends with `.unverified`

#### Scenario: Failed capture
- **WHEN** a generation is written from a capture with the verdict failed
- **THEN** no directory of this run remains in the target

### Requirement: Leftovers are reported and kept
Partial generations left by earlier runs SHALL be reported with their name and age. They SHALL NOT be deleted or modified, because a partial generation may belong to a run that is still in progress.

#### Scenario: Leftover from an aborted run
- **WHEN** the target holds a partial generation from an earlier run
- **THEN** it is reported with its name and age
- **AND** it is still present and unchanged afterwards

### Requirement: The target must already exist
Publishing SHALL require an existing directory as target. A missing target or a target that is not a directory SHALL be refused without creating anything.

#### Scenario: Missing target
- **WHEN** the target path does not exist
- **THEN** publishing is refused
- **AND** the path still does not exist

### Requirement: Owner-only permissions where supported
On platforms with POSIX permissions, a generation directory SHALL be accessible only by its owner, and the archive file SHALL be readable and writable only by its owner.

#### Scenario: Permissions on Linux
- **WHEN** a generation is published on Linux
- **THEN** its directory has mode 0700 and the archive file has mode 0600
