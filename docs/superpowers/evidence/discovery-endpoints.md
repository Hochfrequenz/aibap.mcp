# SAP ADT Discovery Service Endpoints

**Source:** `GET https://srvhfuhana.sap.msp.local:44300/sap/bc/adt/discovery`
**Fetched:** 2026-03-23
**Response size:** ~301 KB

Endpoints are organised by the workspace groups returned in the Atom service document.
A `[USED]` marker indicates an endpoint our MCP server currently calls.

---

## Legend

- `[USED]` – endpoint already called by this MCP server
- `[USED-indirect]` – used implicitly via a pattern (e.g. `{objectURI}/source/main`)
- Collection URIs are the `href` values from `<app:collection>`.
- Template URIs come from nested `<adtcomp:templateLink>` entries.

---

## 1. Discovery / CSRF

| Title | URI | Notes |
|-------|-----|-------|
| ADT Discovery | `/sap/bc/adt/discovery` | `[USED]` – CSRF token fetch + service document |
| ADT HTTP(S) Endpoint | `https://srvhfuhana.sap.msp.local:44300/sap/bc/adt` | Base URL |
| Security Reentranceticket | `/sap/bc/adt/security/reentranceticket` | |

---

## 2. Source Code (Read / Write)

Pattern used by most object types: `{objectUri}/source/main`

| Title | URI | Notes |
|-------|-----|-------|
| Source – GET | `{objectURI}/source/main` | `[USED]` – GetSource |
| Source – PUT | `{objectURI}/source/main` | `[USED]` – SetSource |

Representative collection URIs that expose `/source/main` templates (non-exhaustive):

- `/sap/bc/adt/programs/programs`
- `/sap/bc/adt/programs/includes`
- `/sap/bc/adt/oo/classes`
- `/sap/bc/adt/oo/interfaces`
- `/sap/bc/adt/functions/groups`
- `/sap/bc/adt/ddic/ddl/sources` (CDS views)
- `/sap/bc/adt/ddic/ddlx/sources` (Metadata Extensions)
- `/sap/bc/adt/ddic/ddla/sources` (Annotation Definitions)
- `/sap/bc/adt/ddic/drul/sources` (Dependency Rules)
- `/sap/bc/adt/ddic/dsfd/sources` (Scalar Function Definitions)
- `/sap/bc/adt/ddic/dtdc/sources` (Dynamic Caches)
- `/sap/bc/adt/ddic/dteb/sources` (Entity Buffers)
- `/sap/bc/adt/bo/behaviordefinitions` (RAP Behavior Definitions)
- `/sap/bc/adt/acm/dcl/sources` (DCL Sources)
- `/sap/bc/adt/ddic/srvd/sources` (Service Definitions)
- `/sap/bc/adt/businessservices/eeecevc` (Event Consumption Models)
- `/sap/bc/adt/businessservices/evtbevb` (Event Bindings)
- `/sap/bc/adt/abapdaemons/applications`

---

## 3. Object Properties / Info

| Title | URI | Notes |
|-------|-----|-------|
| Object Properties (GET) | `{objectURI}` | `[USED]` – GetObjectInfo |
| Basic Object Properties | `/sap/bc/adt/vit/wb/object_type/{type}/object_name/{name}` | |
| Object Structure | `/sap/bc/adt/repository/objectstructure` | |

---

## 4. Activation

| Title | URI | Notes |
|-------|-----|-------|
| Activation | `/sap/bc/adt/activation` | `[USED]` – ActivateObject (POST with `?method=activate&preauditRequested=true`) |
| Inactive Objects | `/sap/bc/adt/activation/inactiveobjects` | List / update inactive objects |
| Activation in Background | `/sap/bc/adt/activation/runs` | Async activation runs |
| Activation Result | `/sap/bc/adt/activation/results` | Retrieve async results |

---

## 5. Search / Repository Information

| Title | URI | Notes |
|-------|-----|-------|
| Search | `/sap/bc/adt/repository/informationsystem/search` | `[USED]` – SearchObjects (quick search) |
| Usage References | `/sap/bc/adt/repository/informationsystem/usageReferences` | `[USED]` – WhereUsed |
| Usage Snippets | `/sap/bc/adt/repository/informationsystem/usageSnippets` | |
| Where Used | `/sap/bc/adt/repository/informationsystem/whereused` | Alternative where-used endpoint |
| Executable Objects | `/sap/bc/adt/repository/informationsystem/executableObjects` | |
| Full Name Mapping | `/sap/bc/adt/repository/informationsystem/fullnamemapping` | |
| Meta Data | `/sap/bc/adt/repository/informationsystem/metadata` | |
| Object Types | `/sap/bc/adt/repository/informationsystem/objecttypes` | |
| Release States | `/sap/bc/adt/repository/informationsystem/releasestates` | |
| ABAP Language Versions | `/sap/bc/adt/repository/informationsystem/abaplanguageversions` | |
| Text Search | `/sap/bc/adt/repository/informationsystem/textsearch` | Full-text search |
| Message Search | `/sap/bc/adt/repository/informationsystem/messagesearch` | |
| Virtual Folders | `/sap/bc/adt/repository/informationsystem/virtualfolders` | |
| Virtual Folders Contents | `/sap/bc/adt/repository/informationsystem/virtualfolders/contents` | |
| Object Properties | `/sap/bc/adt/repository/informationsystem/objectproperties/values` | |
| Transport Properties | `/sap/bc/adt/repository/informationsystem/objectproperties/transports` | |
| Object Favorites | `/sap/bc/adt/repository/favorites/lists` | |
| Element Info | `/sap/bc/adt/repository/informationsystem/elementinfo` | |
| Object Sets / References | `/sap/bc/adt/repository/informationsystem/objectsets/references` | |
| Object Sets / Metrics | `/sap/bc/adt/repository/informationsystem/objectsets/metrics` | |

---

## 6. Package / Repository Structure

| Title | URI | Notes |
|-------|-----|-------|
| Node Structure | `/sap/bc/adt/repository/nodestructure` | `[USED]` – BrowsePackage |
| Node Path | `/sap/bc/adt/repository/nodepath` | |
| Type Structure | `/sap/bc/adt/repository/typestructure` | |
| Package | `/sap/bc/adt/packages` | Package CRUD + value helps |
| Package (tree) | `/sap/bc/adt/packages/$tree{?packagename,type}` | Package tree navigation |
| Package Settings | `/sap/bc/adt/packages/settings` | |
| Repository Generators | `/sap/bc/adt/repository/generators` | |
| Proxy URI Mappings | `/sap/bc/adt/repository/proxyurimappings` | |

---

## 7. Syntax / Static Checks

| Title | URI | Notes |
|-------|-----|-------|
| Check Runs | `/sap/bc/adt/checkruns` | `[USED]` – SyntaxCheck (POST) |
| Reporters | `/sap/bc/adt/checkruns/reporters` | List available check reporters |

---

## 8. ABAP Unit Tests

| Title | URI | Notes |
|-------|-----|-------|
| ABAP Unit Testruns | `/sap/bc/adt/abapunit/testruns` | `[USED]` – RunUnitTests (POST) |
| ABAP Unit Metadata | `/sap/bc/adt/abapunit/metadata` | |
| ABAP Unit Testruns Evaluation | `/sap/bc/adt/abapunit/testruns/evaluation` | |
| Test Double Framework – Dependencies | `/sap/bc/adt/aunit/dbtestdoubles/cds/dependencies` | |
| Test Double Framework – Validation | `/sap/bc/adt/aunit/dbtestdoubles/cds/validation` | |

---

## 9. Transport / CTS

| Title | URI | Notes |
|-------|-----|-------|
| Transport Management | `/sap/bc/adt/cts/transportrequests` | `[USED]` – GetTransportRequests (GET) |
| Transport Components | `/sap/bc/adt/cts/transportrequests/{nr}/abaptransportcomponents` | `[USED]` – AddToTransport (POST) |
| Transports | `/sap/bc/adt/cts/transports` | |
| Transport Checks | `/sap/bc/adt/cts/transportchecks` | |
| Transport Reference | `/sap/bc/adt/cts/transportrequests/reference` | |
| Transport Search Configurations | `/sap/bc/adt/cts/transportrequests/searchconfiguration/configurations` | |
| Transport Facets | `/sap/bc/adt/cts/transportrequests/facets` | |

---

## 10. ABAP Test Cockpit (ATC)

| Title | URI | Notes |
|-------|-----|-------|
| ATC Customizing | `/sap/bc/adt/atc/customizing` | |
| ATC Runs | `/sap/bc/adt/atc/runs` | |
| CCS Tunnel | `/sap/bc/adt/atc/ccstunnel` | |
| Result Worklist | `/sap/bc/adt/atc/result/worklist` | |
| Check Failures | `/sap/bc/adt/atc/checkfailures` | |
| Check Failure Logs | `/sap/bc/adt/atc/checkfailures/logs` | |
| ATC Results | `/sap/bc/adt/atc/results` | |
| ATC Worklists | `/sap/bc/adt/atc/worklists` | |
| Autoquickfix | `/sap/bc/adt/atc/autoqf/worklist` | |
| ATC Items | `/sap/bc/adt/atc/items` | |
| ATC Approvers | `/sap/bc/adt/atc/approvers` | |
| ATC Variants | `/sap/bc/adt/atc/variants` | |
| Exemptions | `/sap/bc/adt/atc/exemptions/apply` | |
| Check Categories | `/sap/bc/adt/atc/checkcategories` | |
| Check Exemptions | `/sap/bc/adt/atc/checkexemptions` | |
| Checks | `/sap/bc/adt/atc/checks` | |
| Check Variants | `/sap/bc/adt/atc/checkvariants` | |
| ATC Configuration | `/sap/bc/adt/atc/configuration/configurations` | |

---

## 11. Debugger

| Title | URI | Notes |
|-------|-----|-------|
| Debugger | `/sap/bc/adt/debugger` | |
| Memory Sizes | `/sap/bc/adt/debugger/memorysizes` | |
| System Areas | `/sap/bc/adt/debugger/systemareas` | |
| Breakpoints | `/sap/bc/adt/debugger/breakpoints` | |
| Debugger Listeners | `/sap/bc/adt/debugger/listeners` | |
| Debugger Variables | `/sap/bc/adt/debugger/variables` | |
| Debugger Actions | `/sap/bc/adt/debugger/actions` | |
| Debugger Stack | `/sap/bc/adt/debugger/stack` | |
| Debugger Watchpoints | `/sap/bc/adt/debugger/watchpoints` | |
| Debugger Batch Request | `/sap/bc/adt/debugger/batch` | |

---

## 12. ABAP Source / Code Intelligence

| Title | URI | Notes |
|-------|-----|-------|
| Code Completion | `/sap/bc/adt/abapsource/codecompletion/proposal` | |
| Element Info | `/sap/bc/adt/abapsource/codecompletion/elementinfo` | |
| Code Insertion | `/sap/bc/adt/abapsource/codecompletion/insertion` | |
| Type Hierarchy | `/sap/bc/adt/abapsource/typehierarchy` | |
| Pretty Printer | `/sap/bc/adt/abapsource/prettyprinter` | |
| Pretty Printer Settings | `/sap/bc/adt/abapsource/prettyprinter/settings` | |
| Cleanup | `/sap/bc/adt/abapsource/cleanup/source` | |
| Occurrence Markers | `/sap/bc/adt/abapsource/occurencemarkers` | |
| ABAP Doc Export | `/sap/bc/adt/abapsource/abapdoc/exportjobs` | |
| ABAP Syntax Configurations | `/sap/bc/adt/abapsource/syntax/configurations` | |
| Navigation | `/sap/bc/adt/navigation/target` | Go-to-definition / navigation |
| Navigation Index Update | `/sap/bc/adt/navigation/indexupdate` | |
| Quickfixes | `/sap/bc/adt/quickfixes/evaluation` | |
| Refactoring | `/sap/bc/adt/refactorings` | |
| Change Package Assignment | `/sap/bc/adt/refactoring/changepackage` | |

---

## 13. Dictionary (DDIC)

| Title | URI | Notes |
|-------|-----|-------|
| Data Elements | `/sap/bc/adt/ddic/dataelements` | |
| Domains | `/sap/bc/adt/ddic/domains` | |
| Structures | `/sap/bc/adt/ddic/structures` | |
| Database Tables | `/sap/bc/adt/ddic/tables` | |
| Table Types | `/sap/bc/adt/ddic/tabletypes` | |
| Table Indexes | `/sap/bc/adt/ddic/db/indexes` | |
| Technical Table Settings | `/sap/bc/adt/ddic/db/settings` | |
| Lock Objects | `/sap/bc/adt/ddic/lockobjects/sources` | |
| Type Groups | `/sap/bc/adt/ddic/typegroups` | |
| Views (External) | `/sap/bc/adt/ddic/views` | |
| Extension Indexes | `/sap/bc/adt/ddic/extensionindexes` | |
| Element Info | `/sap/bc/adt/ddic/elementinfo` | |
| Code Completion | `/sap/bc/adt/ddic/codecompletion` | |
| Activation Graph | `/sap/bc/adt/ddic/logs/activationgraph` | |
| DB Procedure Proxies | `/sap/bc/adt/ddic/dbprocedureproxies` | |
| DDIC Validation (SQSC) | `/sap/bc/adt/ddic/validation` | |

---

## 14. CDS / DDL

| Title | URI | Notes |
|-------|-----|-------|
| DDL Sources | `/sap/bc/adt/ddic/ddl/sources` | CDS views |
| DDL Parser | `/sap/bc/adt/ddic/ddl/parser` | |
| DDL Dependency Analyzer | `/sap/bc/adt/ddic/ddl/dependencies/graphdata` | |
| DDL Element Info | `/sap/bc/adt/ddic/ddl/elementinfo` | |
| DDL Element Mappings | `/sap/bc/adt/ddic/ddl/elementmappings` | |
| DDL Active Object | `/sap/bc/adt/ddic/ddl/activeobject` | |
| DDL Related Objects | `/sap/bc/adt/ddic/ddl/relatedObjects` | |
| DDL Migration | `/sap/bc/adt/ddic/ddl/migration/bgruns` | |
| DDL Language Help | `/sap/bc/adt/docu/ddl/langu` | |
| DDL Formatter | `/sap/bc/adt/ddic/ddl/formatter/identifiers` | |
| DDL Formatter Configurations | `/sap/bc/adt/ddic/ddl/formatter/configurations` | |
| DDLA Sources | `/sap/bc/adt/ddic/ddla/sources` | Annotation Definitions |
| DDLX Sources | `/sap/bc/adt/ddic/ddlx/sources` | Metadata Extensions |
| Service Definition | `/sap/bc/adt/ddic/srvd/sources` | |
| Annotation Definitions | `/sap/bc/adt/ddic/cds/annotation/definitions` | |
| Aspect Sources | `/sap/bc/adt/ddic/dras/sources` | |
| Dependency Rule Sources | `/sap/bc/adt/ddic/drul/sources` | |
| Scalar Function Definition | `/sap/bc/adt/ddic/dsfd/sources` | |
| Scalar Function Implementation | `/sap/bc/adt/ddic/dsfi` | |
| Dynamic Cache Sources | `/sap/bc/adt/ddic/dtdc/sources` | |
| Entity Buffer Sources | `/sap/bc/adt/ddic/dteb/sources` | |
| CDS Type Sources | `/sap/bc/adt/ddic/drty/sources` | |

---

## 15. Business Services (RAP / OData)

| Title | URI | Notes |
|-------|-----|-------|
| Service Binding | `/sap/bc/adt/businessservices/bindings` | |
| Service Consumption Model | `/sap/bc/adt/businessservices/consmodels` | |
| Event Consumption Model | `/sap/bc/adt/businessservices/eeecevc` | |
| Event Binding | `/sap/bc/adt/businessservices/evtbevb` | |
| OData V4 | `/sap/bc/adt/businessservices/odatav4` | |
| OData V2 | `/sap/bc/adt/businessservices/odatav2` | |
| Service Binding Classification | `/sap/bc/adt/businessservices/release` | |
| RAP Behavior Definition | `/sap/bc/adt/bo/behaviordefinitions` | |
| BOPF Business Objects | `/sap/bc/adt/bopf/businessobjects` | |

---

## 16. Programs / Function Groups / Classes

| Title | URI | Notes |
|-------|-----|-------|
| Programs | `/sap/bc/adt/programs/programs` | |
| Includes | `/sap/bc/adt/programs/includes` | |
| Run a Program | `/sap/bc/adt/programs/programrun` | |
| Function Groups | `/sap/bc/adt/functions/groups` | |
| Classes | `/sap/bc/adt/oo/classes` | |
| Interfaces | `/sap/bc/adt/oo/interfaces` | |
| Run a Class | `/sap/bc/adt/oo/classrun` | |
| Text Elements (Programs) | `/sap/bc/adt/textelements/programs` | |
| Text Elements (Function Groups) | `/sap/bc/adt/textelements/functiongroups` | |
| Text Elements (Classes) | `/sap/bc/adt/textelements/classes` | |
| Message Classes | `/sap/bc/adt/messageclass` | |

---

## 17. Object Lifecycle (Lock / Delete / Classification)

| Title | URI | Notes |
|-------|-----|-------|
| Deletion | `/sap/bc/adt/deletion/delete` | |
| Deletion Check | `/sap/bc/adt/deletion/check` | |
| API Releases | `/sap/bc/adt/apireleases` | Release state management |
| Classifications | `/sap/bc/adt/classifications` | |
| Object Relations | `/sap/bc/adt/objectrelations` | |

---

## 18. System Information

| Title | URI | Notes |
|-------|-----|-------|
| System Information | `/sap/bc/adt/system/information` | |
| Installed Components | `/sap/bc/adt/system/components` | |
| System Clients | `/sap/bc/adt/system/clients` | |
| System Landscape | `/sap/bc/adt/system/landscape/servers` | |
| User | `/sap/bc/adt/system/users` | User search/info |

---

## 19. Enhancements / BAdI

| Title | URI | Notes |
|-------|-----|-------|
| Enhancement Implementation | `/sap/bc/adt/enhancements/enhoxh` | |
| BAdI Implementation | `/sap/bc/adt/enhancements/enhoxhb` | |
| Source Code Plugin | `/sap/bc/adt/enhancements/enhoxhh` | |
| Enhancement Spot | `/sap/bc/adt/enhancements/enhsxs` | |
| BAdI Enhancement Spot | `/sap/bc/adt/enhancements/enhsxsb` | |
| BAdIs | `/sap/bc/adt/businesslogicextensions/badis` | |

---

## 20. Data Preview

| Title | URI | Notes |
|-------|-----|-------|
| DDIC Data Preview | `/sap/bc/adt/datapreview/ddic` | |
| CDS Data Preview | `/sap/bc/adt/datapreview/cds` | |
| Freestyle Data Preview | `/sap/bc/adt/datapreview/freestyle` | |
| AMDP Data Preview | `/sap/bc/adt/datapreview/amdp` | |
| AMDP Debugger Data Preview | `/sap/bc/adt/datapreview/amdpdebugger` | |

---

## 21. Performance / Profiling

| Title | URI | Notes |
|-------|-----|-------|
| Performance Trace State | `/sap/bc/adt/st05/trace/state` | |
| Performance Trace Directory | `/sap/bc/adt/st05/trace/directory` | |
| ABAP Profiler Trace Files | `/sap/bc/adt/runtime/traces/abaptraces` | |
| ABAP Profiler Requests | `/sap/bc/adt/runtime/traces/abaptraces/requests` | |
| Dynamic Logpoints | `/sap/bc/adt/dlp/logpoints` | |
| Logpoint Logs | `/sap/bc/adt/dlp/logs/servers` | |
| AMDP Debugger | `/sap/bc/adt/amdp/debugger/main` | |
| Work Processes | `/sap/bc/adt/runtime/workprocesses` | |
| SQLM Data | `/sap/bc/adt/sqlm/data` | |

---

## 22. Connectivity (AMC / APC / HTTP Services)

| Title | URI | Notes |
|-------|-----|-------|
| ABAP Messaging Channel | `/sap/bc/adt/uc_object_type_group/samc` | |
| ABAP Push Channel Application | `/sap/bc/adt/uc_object_type_group/sapc` | |
| HTTP Service | `/sap/bc/adt/ucon/httpservices` | |

---

## 23. Miscellaneous Object Types

| Title | URI | Notes |
|-------|-----|-------|
| ABAP Daemon | `/sap/bc/adt/abapdaemons/applications` | |
| Application Job Catalog | `/sap/bc/adt/applicationjob/catalogs` | |
| Application Job Template | `/sap/bc/adt/applicationjob/templates` | |
| Application Log Object | `/sap/bc/adt/applicationlog/objects` | |
| Knowledge Transfer Doc | `/sap/bc/adt/documentation/ktd/documents` | |
| Number Range Object | `/sap/bc/adt/numberranges/objects` | |
| Archiving Object | `/sap/bc/adt/archivingobjects/objects` | |
| Change Document Object | `/sap/bc/adt/changedocuments/objects` | |
| Transport Object Definition | `/sap/bc/adt/transportobject/objects` | |
| Metric Provider | `/sap/bc/adt/metricproviders` | |
| Custom Field | `/sap/bc/adt/customfields/objects` | |
| Situation Object | `/sap/bc/adt/sit/sitotyp` | |
| Switch-Based Feature Toggle | `/sap/bc/adt/sfw/featuretoggles` | |
| Feature Toggle | `/sap/bc/adt/lifecycle_management/ftglaf` | |
| SAP Object Node Type | `/sap/bc/adt/businessobjects/nontnot` | |
| SAP Object Type | `/sap/bc/adt/businessobjects/rontrot` | |
| Web Dynpro Components | `/sap/bc/adt/wdy/components` | |
| Web Dynpro Applications | `/sap/bc/adt/wdy/applications` | |
| HDI Artifact | `/sap/bc/adt/hota/hotahto` | |
| HDI Namespace | `/sap/bc/adt/hota/hotahdi` | |
| URI Fragment Mapper | `/sap/bc/adt/urifragmentmappings` | |
| VIT URI Mapper | `/sap/bc/adt/vit/urimapper` | |
| Navigation | `/sap/bc/adt/navigation/target` | |
| Feeds | `/sap/bc/adt/feeds` | |

---

## 24. API Management (APS)

| Title | URI | Notes |
|-------|-----|-------|
| API Package | `/sap/bc/adt/aps/com/sod1` | |
| API Package Assignment | `/sap/bc/adt/aps/com/sod2` | |
| Authorization Field | `/sap/bc/adt/aps/iam/auth` | |
| Authorization Default Values | `/sap/bc/adt/aps/iam/sush` | |
| Authorization Object | `/sap/bc/adt/aps/iam/suso` | |
| Technical Object Group | `/sap/bc/adt/aps/common/sbc1` | |

---

## Summary: Current Coverage vs. Available Endpoints

| Functional Area | Endpoints Used | Endpoints Available |
|----------------|---------------|-------------------|
| Discovery / CSRF | 1 | 3 |
| Source Read/Write | 2 | ~40+ object types |
| Object Properties | 1 | 3 |
| Activation | 1 | 4 |
| Search | 1 | 15+ |
| Where Used | 1 | 2 |
| Package Browse | 1 | 8 |
| Syntax Check | 1 | 2 |
| Unit Tests | 1 | 4 |
| Transport (CTS) | 2 | 7 |
| ATC | 0 | 19 |
| Debugger | 0 | 10 |
| Code Intelligence | 0 | 10+ |
| DDIC | 0 | 20+ |
| CDS/DDL | 0 | 20+ |
| Business Services | 0 | 9 |
| Data Preview | 0 | 5 |
| System Info | 0 | 5 |
| Profiling/Tracing | 0 | 9 |
| **Total (approx.)** | **12** | **200+** |

---

## Notable Endpoints Not Yet Used

These represent the highest-value gaps for MCP tool expansion:

| Endpoint | Potential MCP Tool |
|----------|--------------------|
| `GET /sap/bc/adt/navigation/target` | Go-to-definition / object navigation |
| `POST /sap/bc/adt/abapsource/prettyprinter` | Format ABAP source |
| `GET /sap/bc/adt/abapsource/codecompletion/proposal` | Code completion |
| `GET /sap/bc/adt/abapsource/codecompletion/elementinfo` | Hover / element info |
| `GET /sap/bc/adt/abapsource/typehierarchy` | Type hierarchy |
| `GET /sap/bc/adt/quickfixes/evaluation` | Quick fix suggestions |
| `POST /sap/bc/adt/refactorings` | Rename / refactor |
| `GET /sap/bc/adt/atc/runs` + `POST /sap/bc/adt/atc/worklists` | ATC code quality checks |
| `GET /sap/bc/adt/repository/informationsystem/textsearch` | Full-text source search |
| `GET /sap/bc/adt/datapreview/ddic` | Table / CDS data preview |
| `GET /sap/bc/adt/system/information` | System metadata |
| `DELETE /sap/bc/adt/deletion/delete` | Object deletion |
| `GET /sap/bc/adt/packages/$tree` | Package tree navigation |
| `GET /sap/bc/adt/activation/inactiveobjects` | List inactive (unsaved) objects |
| `GET /sap/bc/adt/apireleases/{uri}` | API release state |
