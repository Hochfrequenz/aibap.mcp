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
| **Self-contained** | **A**: standard loop against the target system | **C**: standard loop on another system with ADT, deliver by transport | **C** |
| **Embedded** | **B**: standard loop in the existing system, git tracks the change set | **D**: AI-assisted manual work | **E**: no workable route |

- **A**: develop in the target system's development system through ADT, as in
  section 2. Deliver through that system's own transport landscape.
- **B**: like A, but start from what is in the system: export or read the
  current state into git first (`export_package`, `get_source`); from then on
  git is authoritative for the change. The unit of change is the transport
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

## 2. The standard loop (A, B, C)

```
branch → write files → (optional) run logic locally without SAP
       → create_object / set_source_from_file
       → activate_objects · run_unit_tests · run_atc_check
           ↺ on errors: fix the file and write again
       → export_package → diff against the branch
       → commit → pull request → human review → merge → tag
       → deliver (section 4)
```

- Files in git, in abapGit layout, are the working copy. ADT writes them into
  SAP and runs the checks there.
- `export_package` writes a package in abapGit layout to disk, without abapGit
  online and without an internet connection on the SAP side. It needs the
  companion package named in its tool description installed on the system.
- A fix made directly in SAP (for example during a debugging session) is
  allowed, but export and commit it right away, otherwise SAP and git drift
  apart.
- Where abapGit is installed and the SAP system can reach the git host,
  `abapgit_pull` / `abapgit_push` are an alternative to the ADT write and the
  export. They need the companion endpoints named in their tool descriptions.

## 3. Rules

**Must**

1. **git is the source of truth.** A human reviews the change in a pull
   request before it is delivered.
2. **Compatibility with the lowest target release is checked before
   delivery.** How is up to you: develop on that release, activate there, or
   at least run a static check configured for its syntax version (for
   example abaplint).
3. **Document what could not be checked**, in the pull request or the
   release notes.

**Should**

4. **Compare SAP with git before a release or delivery**: export, diff, and
   expect no differences. Fully possible in A and C. In B only for the objects
   of your own request (others may have changed the same objects). In D only
   by inspection. In E not at all.
5. **A commit is the checkpoint**, not a transport release. Use
   `rollback_transport` as a last resort (see its limits below).
6. **Run the inner loop without SAP where possible**: unit tests with test
   doubles, or a local ABAP interpreter. Results from there do not replace the
   checks in SAP.
7. **One workbench request per feature.** For self-contained work, one
   package per repository.

When to release a transport is a governance decision of the team or the
system owner, not of the tooling.

## 4. Delivery

| Situation | Route |
|---|---|
| Target has ADT access | Write the tagged state into its development system through ADT, then use its transport landscape. |
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
- **Transport of copies and customizing requests**: `create_transport`
  creates them (category `T` or `W`) over ADT on S/4HANA. On ECC it does so
  only if your build includes a fallback; otherwise create them in SE09. No
  tool can fill a transport of copies yet
  ([#602](https://github.com/Hochfrequenz/aibap.mcp/issues/602)).
- **`create_object`**: DDIC and CDS types (TABL, DTEL, DOMA, DDLS) only on
  S/4HANA, unless your build includes a fallback (TABL, DTEL, DOMA).
- **`export_package` / `export_packages`** export whole packages. There is no
  export of the objects of a single transport request yet, which limits the
  comparison in B.
