# Workspaces

The Workspaces domain models workspaces and the Users who are their members.
A Workspace groups [Users](users.md#user) under a shared name. Each member
has a Workspace role, which decides what they may do in that Workspace.

Workspace roles are separate from the system-wide
[Roles and Permissions](users.md), which govern the admin panel rather than
what a member does inside a Workspace.

## Model structure

| Concept | Classification | Boundary and relationships |
| --- | --- | --- |
| Workspace | Entity and aggregate root | Owns a name and its memberships; references Users only by identity. |
| Membership | Value object | Records that one User belongs to one Workspace with a Workspace role; owned by that Workspace. |
| Workspace role | Value object | Names what a member may do in the Workspace. |
| Workspace identity | Value object | Identifies a Workspace independently of its name and members. |
| Workspace name | Value object | Names a Workspace in plain, untranslated text. |

```mermaid
flowchart LR
    subgraph WorkspaceAggregate[Workspace aggregate]
        Workspace[Workspace] -->|owns| Name[Workspace name]
        Workspace -->|owns zero or more| Membership[Memberships]
        Membership -->|has one| Role[Workspace role]
    end
    Membership -->|references one| User[User aggregate]
```

## Workspace aggregate

A Workspace is a named group of Users. Its identity remains the same when its
name or members change. The Workspace aggregate contains its memberships; the
Users they reference have their own lifecycle and are outside the aggregate.

| Field | Domain value | Required | Meaning and rules |
| --- | --- | --- | --- |
| Identity | Workspace identity | Yes | Distinguishes this Workspace from all other Workspaces; nonempty, assigned on creation, and stable. |
| Name | Workspace name | Yes | Names the Workspace; need not be unique among Workspaces. |
| Members | Collection of memberships | Yes; may be empty | Lists the Users who belong to the Workspace, at most once each; unordered. |

Renaming a Workspace changes only its name. Adding or removing a member, or
changing a member's role, changes only its memberships.

A User who creates a Workspace becomes its owner. A Workspace created without
a creator, such as from the admin panel, starts with no members.

Deleting a Workspace removes all of its memberships; the Users who were its
members remain.

### Membership

A Membership records that a User belongs to a Workspace. It has no identity of
its own: the Workspace and User it relates determine it, and two Memberships
relating the same Workspace and User are the same Membership.

| Field | Domain value | Required | Meaning and rules |
| --- | --- | --- | --- |
| Workspace | Workspace identity | Yes | Identifies the Workspace that owns the Membership. |
| User | [User identity](users.md#account-and-authority-values) | Yes | References one existing User by identity; the User is not part of the Workspace aggregate. |
| Role | Workspace role | Yes | Decides what the User may do in the Workspace. A member added without a role is a viewer. |

### Workspace role

A Workspace role is one of three values, from the least to the most
authority:

| Value | Meaning |
| --- | --- |
| `viewer` | Views the Workspace's content. |
| `editor` | Views and edits the Workspace's content. |
| `owner` | Also renames the Workspace and manages its members. |

Each role allows these actions:

| Action | viewer | editor | owner |
| --- | --- | --- | --- |
| View workspace content | yes | yes | yes |
| Edit workspace content | no | yes | yes |
| Rename the workspace | no | no | yes |
| Add, remove, or change the role of members | no | no | yes |

An action a member's role does not allow is denied. A User who is not a
member cannot tell the Workspace from a missing one.

## Value objects

| Value object | Field | Meaning and rules |
| --- | --- | --- |
| Workspace identity | Identity value | Nonempty identity of one Workspace. |
| Workspace role | Role value | One of `viewer`, `editor`, or `owner`; see [Workspace role](#workspace-role) for what each allows. |
| Workspace name | Name text | Plain text that is not translated. Leading and trailing whitespace is removed from a supplied name; the result is valid text with a non-whitespace character, has no leading or trailing whitespace, and is at most 100 characters. |

## Aggregate invariants

- A Workspace has at most one Membership for any User.
- A Workspace may have no members.
- A Workspace that has an owner always keeps at least one: removing or
  demoting its last owner fails.
- Every Membership belongs to exactly one Workspace and ends when that
  Workspace is deleted.

## Invariants across aggregates

- Every Membership references an existing User.
- A User may be a member of any number of Workspaces, including none.
- Deleting a User removes that User's Memberships from every Workspace and
  leaves the Workspaces, including any left with no members or with no
  owner.
