# Development workflow guide

How to organise an ABAP development project with this server: where the source
of truth lives, how code gets into SAP and back out, and how it is delivered.
Read this before starting a project. The tool descriptions cover single steps;
this guide covers how they fit together.

Tools named here may be disabled by the server's tool-group configuration.
Rules are marked **must** or **should**. A *should* that cannot be met in a
given setup is acceptable, but say so in the pull request or release notes.

## 1. Two questions first

Answer these before anything else. The rest of the workflow depends on them.

1. **Is the change self-contained, or embedded in existing custom code?**
   - *Self-contained*: its own package, depending only on SAP standard objects
     of the target release.
   - *Embedded*: it extends or calls the system's existing custom objects, its
     enhancements (BAdI implementations, exits), its customizing or its data.
2. **What access is there to the system the code is written for?**
   ADT (this server works), SAP GUI only, or none (transports are imported by
   someone else).

| | ADT access | SAP GUI only | No access |
|---|---|---|---|
| **Self-contained** | **A**: loop (section 2) against the target system | **C**: loop (section 2) on another system with ADT, deliver by transport | **C** |
| **Embedded** | **B**: loop (section 2) in the existing system, git tracks the change set | **D**: AI-assisted manual work | **E**: no workable route |

- **A**: develop in the target system's development system through ADT, with the loop in
  section 2. Deliver through that system's own transport landscape.
- **B**: like A, but start from what is in the system: export or read the
  current state into git first (`export_package`, or `get_source` without
  the export companion) so the diff shows only your change. The unit of change is the transport
  request, not the package, because changes usually touch objects in several
  packages that other people also work on. Code that depends heavily on the
  existing system cannot be meaningfully tested outside it.
- **C**: develop on a system you do have ADT access to, at the **lowest
  release** among the targets (or check compatibility as in rule 2 below).
  Deliver as a transport: put the objects into a request, release it, and
  hand the transport files (outside this server) to whoever imports them. Or
  apply the code by hand. On an ECC system the release step needs SE09 or a
  fallback (section 5).
- **D**: write the code locally, then apply and test it through SAP GUI (for
  example with the [sapgui.mcp](https://github.com/Hochfrequenz/sapgui.mcp)
  server). Commit what was applied.
- **E**: code that depends on a system nobody can reach cannot be built or
  tested with confidence. Say so instead of guessing.

## 2. The loop (A, B, C)

Two variants of the same loop. Both end the same way: commit, pull request,
review, deliver. Pick one per change with the table below.

**SAP-first**: SAP is the working copy, git is the record.

```
branch → edit in SAP through ADT: create_object, get_source,
         patch_source / set_source_from_file
       → activate_objects · run_unit_tests · run_atc_check   ↺ until green
       → export_package into the branch → git diff shows the change
       → commit → pull request → human review → merge → tag → deliver
```

**Files-first**: the files are the working copy, SAP runs the checks.

```
branch → write files → test locally without SAP (unit tests with test
         doubles, or a local ABAP interpreter)
       → create_object / set_source_from_file
       → activate_objects · run_unit_tests · run_atc_check
           ↺ on errors: fix the file and write again
       → export_package → diff against the branch (if available)
       → commit → pull request → human review → merge → tag → deliver
```

**Which variant**

Decide in two steps. Step 1 can force a variant for the whole change; if it
does not, step 2 decides object by object, and the first matching row wins.

*Step 1: the whole change*

| Situation | Variant | Why |
|---|---|---|
| The export companion (see `export_package`) is not installed on the development system and cannot be | Files-first | Without the export, code reaches git only from files; git then holds the sources, not SAP's full serialization. |
| Several branches touch the same objects in parallel | Files-first for the branch work | SAP holds one state per object; write into SAP one branch at a time. |

*Step 2: each object*

| Object | Variant | Why |
|---|---|---|
| Logic that can be isolated and tested without SAP, even inside an embedded change (e.g. a new helper class) | Files-first | Fast local loop; SAP only for the final checks. |
| DDIC object | SAP-first | DDIC is created in SAP (on ECC see section 5); the export serializes it. |
| Code that depends on the existing system (case B) | SAP-first | It can only be tested in the system anyway, and the export captures exactly what SAP has. |
| Anything else | SAP-first | Fewer sync steps, and SAP does the serialization. |

Both variants can be mixed within one change, object by object, with two
rules:

- **One working copy per object at a time.** Between two exports, edit an
  object either locally or in SAP, not both. An export overwrites the local
  file, and writing a file overwrites SAP.
- **In SAP-first, export at every green state** and before ending a
  session, including after fixes made while debugging.

Before relying on SAP-first, check once on the system that an export without
changes produces an empty `git diff`. Otherwise every export shows noise
that hides the real change.

`export_package` needs no abapGit online and no internet connection on the
SAP side, but the companion package named in its tool description must be
installed. Where abapGit is installed and the SAP system can reach the git
host, `abapgit_pull` / `abapgit_push` are an alternative to the ADT write and
the export; they need the companion endpoints named in their tool
descriptions.

## 3. Rules

**Must**

1. **What is delivered is a reviewed git commit.** git is the source of
   truth for every delivery: a human reviews the change in a pull request
   first, whichever loop variant produced it.
2. **Compatibility with the lowest target release is checked before
   delivery.** How is up to you: develop on that release, activate there, or
   at least run a static check configured for its syntax version (for
   example abaplint).
3. **Document what could not be checked**, in the pull request or the
   release notes.

**Should**

4. **Compare SAP with git before a release or delivery**: export, diff, and
   expect no differences, i.e. nothing changed in SAP since the reviewed
   commit. Needs the export companion. Fully possible in A and C. In B only
   for the objects of your own request (others may have changed the same
   objects). In D only by inspection. In E not at all.
5. **A commit is the checkpoint**, not a transport release. Use
   `rollback_transport` as a last resort (see its limits below).
6. **Use local tests for isolated logic** (section 2). They do not replace
   the checks in SAP.
7. **One workbench request per feature.** For self-contained work, one
   package per repository.

When to release a transport is a governance decision of the team or the
system owner, not of the tooling.

## 4. Delivery

| Situation | Route |
|---|---|
| Target has ADT access | SAP-first: the tagged state should already be in its development system. Files-first: write it there through ADT. Compare (rule 4), then use its transport landscape. |
| Target has no ADT access | Build a transport from the tagged state on your own system (compare first, rule 4), release it, hand over the files. Or apply by hand. |

A transport built from a reviewed and compared git state is as trustworthy as
any manually built one. Imports can still fail on the target (missing
dependencies, release differences), exactly as with manual delivery.

## 5. Tool limits that matter for the workflow

- **`release_transport`**: works over ADT on S/4HANA. On ECC, release over
  ADT does not work (SAP can answer success while the request stays
  modifiable; the tool detects this). Release through SE09 unless your build
  includes a fallback.
- **`rollback_transport`**: restores the version before the transport for
  source objects (PROG, CLAS, INTF, FUGR) only; other types are skipped.
  Objects the transport *created* are reported as failed and left in place
  ([#603](https://github.com/Hochfrequenz/aibap.mcp/issues/603)).
- **Transport of copies**: `create_transport` can create one (category `T`),
  but no tool can fill it yet ([#602](https://github.com/Hochfrequenz/aibap.mcp/issues/602)).
- **`create_object`**: DDIC and CDS types (TABL, DTEL, DOMA, DDLS) only on
  S/4HANA, unless your build includes a fallback (TABL, DTEL, DOMA).
- **`export_package` / `export_packages`** export whole packages. There is no
  export of the objects of a single transport request yet, which limits the
  comparison in B.
