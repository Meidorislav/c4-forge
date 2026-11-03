# ADR-0006: Workspace ownership and last-owner protection

- **Status:** Accepted
- **Date:** 2026-11-03
- **Related requirements:** WS-1, WS-2, AU-3, AU-4, NF-9, NF-10
- **Builds on:** ADR-0003, ADR-0004
- **Supersedes:** ADR-0004's deferral of ownership transfer and last-owner protection; its other rules remain in effect.

## Context

ADR-0004 requires ownership transfer and last-owner protection to be specified
before M4 is implemented. M4/4.1 creates owner memberships; M4/4.2 adds member
management. Disabling an account in the existing administration API must not
leave a workspace without an enabled owner. Instance administrators do not gain
workspace access through their instance role.

## Options considered

1. Exactly one owner, replaced through an atomic transfer. Simple, but prevents
   shared responsibility and makes recovery depend on a single account.
2. One or more owners, with atomic transfer and protection of the last enabled
   owner. Allows shared responsibility but requires transactional checks.
3. Allow ownerless workspaces and give administrators implicit recovery access.
   Conflicts with ADR-0004's explicit-membership boundary. Rejected.

## Decision

Choose **option 2**.

### Permissions and transfer

- Any enabled user whose mandatory password change is complete may create a
  workspace. Creation and the creator's owner membership commit together.
- A new workspace is visible only to its creator. Every later read, list,
  search and sort includes only workspaces where the caller is a member.
- Owners may rename the workspace, edit its description and delete it. Editors
  may edit model data but cannot change workspace metadata or delete it.
  Viewers have read access only. Instance admin status grants no additional
  workspace permissions.
- Multiple enabled owners are allowed. Only an owner may grant ownership.
- An ownership transfer promotes an existing enabled member to owner and
  demotes the initiating owner to editor in one transaction. The recipient
  must have completed any mandatory password change. No intermediate ownerless
  state is committed. The recipient does not need to accept the transfer.
- An owner may instead promote another eligible member and retain ownership.
  Removing or demoting an owner, including oneself, is allowed only when at
  least one other enabled owner remains.

### Last-owner protection and account lifecycle

- Every workspace created through the application must retain at least one
  enabled owner until the workspace itself is deleted. Disabled memberships
  do not count toward this invariant.
- Reject disabling a user who is the last enabled owner of any workspace,
  including when an instance administrator requests it. Return a conflict
  without disabling the account or revoking its sessions. The administrator
  must arrange a transfer or an additional owner first; there is no implicit
  ownership assignment or administrator bypass.
- Password reset does not disable the account or remove ownership. An owner
  with a mandatory password change must complete it before workspace access.
- Deleting a workspace is an explicit owner-only operation that removes its
  stored model, history and memberships together. The UI requires confirmation.
  Last-owner protection does not prevent deleting the workspace itself.

### Transaction ordering

- Serialize account-disable decisions and membership lifecycle changes through
  the existing account-administration transaction lock, acquired first. Lock
  affected workspace rows in stable ID order before checking or changing owners.
  Creation uses the same ordering and rechecks that its creator is enabled.
- Check the caller's authorization, recipient eligibility and remaining enabled
  owner count within the transaction that commits the change. Concurrent
  transfers, removals and account disabling cannot each pass a stale check.
- Model writes continue to use ADR-0003's workspace row lock. They do not acquire
  the account-administration lock after acquiring a workspace lock.
- Reject a last-owner violation with HTTP 409 and a stable error code. An
  administrative response must not disclose names, descriptions or model data
  from workspaces the administrator cannot access.

### Roadmap boundary

- **4.1:** implement workspace metadata and owner membership, owner-only metadata
  mutations/deletion, membership-filtered listing, and the account-disable guard.
  Search covers name and description; default order is most recently modified
  first, with a stable ID tie-breaker. Metadata and persistent model changes
  advance the last-modified timestamp; reads do not.
- **4.2:** implement member management, ownership promotion/transfer, and
  removal/demotion guards. Specifying them here does not bring their API or UI
  into 4.1.
- **4.3:** define authorization revocation ordering for in-flight realtime
  operations as already required by ADR-0004.
- Existing model-store workspace rows have no memberships or creator identity.
  Do not infer ownership from operation actors or assign them to an arbitrary
  administrator. Such rows remain inaccessible through the workspace API;
  retain their data and document this migration behavior in 4.1.

## Consequences

- Account disabling may require coordination with workspace owners first.
  Recovery from an unavailable sole owner remains an explicit operator procedure,
  outside the normal administrator API; no automatic privilege escalation is added.
- A global lifecycle lock is simple at the documented instance scale but
  serializes ownership changes and account disabling across workspaces.
- Owner lifecycle endpoints remain deferred to 4.2. During 4.1 a sole creator
  cannot be disabled while their workspace exists.

## Verification required

- Creation commits workspace and membership together and cannot race with disabling
  its creator to produce a workspace without an enabled owner.
- Nonmembers, including instance administrators, cannot discover or mutate
  workspace data. Editors and viewers cannot change metadata or delete workspaces.
- Last-owner account-disable conflicts are atomic and do not revoke sessions.
- Concurrent ownership changes and account disabling preserve an enabled owner;
  transfer either commits both role changes or neither (4.2).
- Workspace deletion removes dependent data atomically. Search and sorting never
  include inaccessible workspaces.
