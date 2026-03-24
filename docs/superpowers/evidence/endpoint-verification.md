# Endpoint Verification Results

**System:** srvhfuhana.sap.msp.local:44300
**Date:** 2026-03-23
**Client:** 100
**User:** kleink

---

## Test 1: GetSource

**Endpoint:** `GET /sap/bc/adt/programs/programs/RSPARAM/source/main`
**Accept:** `text/plain`
**Status:** 200 OK
**Content-Type:** `text/plain; charset=utf-8`
**ETag:** `202308031506360011`
**Last-Modified:** Thu, 03 Aug 2023 15:06:36 GMT

**Response (full body):**
```abap
REPORT RSPARAM LINE-SIZE 254.
PARAMETERS: ALSOUSUB AS CHECKBOX DEFAULT ' '.
SUBMIT RSPFPAR.

*Commented as ALSOUSUB is not the parameter to be passed, ATC prio 1 error
*WITH ALSOUSUB EQ ALSOUSUB.
```

**Match with our code:** YES -- Our `GetSource` uses `doRead` (GET) and reads the response body as text. The ETag is returned in the header as expected. This works correctly.

---

## Test 2: SearchObjects

**Endpoint:** `GET /sap/bc/adt/repository/informationsystem/search?operation=quickSearch&query=CL_*&maxResults=5`
**Accept:** `application/xml`
**Status:** 200 OK
**Content-Type:** `application/xml; charset=utf-8`

**Response (full body):**
```xml
<?xml version="1.0" encoding="utf-8"?>
<adtcore:objectReferences xmlns:adtcore="http://www.sap.com/adt/core">
  <adtcore:objectReference adtcore:uri="/sap/bc/adt/oo/classes/cl_1031_hrpayit_inail"
    adtcore:type="CLAS/OC" adtcore:name="CL_1031_HRPAYIT_INAIL"
    adtcore:packageName="PB15" adtcore:description="BAdI class: HRPAYIT_INAIL"/>
  <adtcore:objectReference adtcore:uri="/sap/bc/adt/programs/includes/cl_16acc_app"
    adtcore:type="PROG/I" adtcore:name="CL_16ACC_APP"
    adtcore:packageName="J3RF" adtcore:description="Include CL_16ACC_APP"/>
  <adtcore:objectReference adtcore:uri="/sap/bc/adt/oo/classes/cl_1ig_ce_isd_ex"
    adtcore:type="CLAS/OC" adtcore:name="CL_1IG_CE_ISD_EX"
    adtcore:packageName="J1I_GST_LO"
    adtcore:description="GST IN CE: Badi example imp. for Input Service Distribution"/>
  <adtcore:objectReference adtcore:uri="/sap/bc/adt/oo/classes/cl_1ig_ce_jv_ex"
    adtcore:type="CLAS/OC" adtcore:name="CL_1IG_CE_JV_EX"
    adtcore:packageName="J1I_GST_LO"
    adtcore:description="GST IN CE: Example Imp. for JV posting"/>
  <adtcore:objectReference adtcore:uri="/sap/bc/adt/oo/classes/cl_1order_table_extension_cntx"
    adtcore:type="CLAS/OC" adtcore:name="CL_1ORDER_TABLE_EXTENSION_CNTX"
    adtcore:packageName="CRM_BTX_EXTENSION"/>
</adtcore:objectReferences>
```

**Match with our code:** YES -- Our `xmlObjectReferences` struct expects `<objectReferences>` with child `<objectReference>` elements having `uri`, `type`, `name`, `description`, `packageName` attributes. The real response uses `adtcore:` namespace prefix on everything. Go's `encoding/xml` ignores namespace prefixes on attributes by default, so `xml:"uri,attr"` matches `adtcore:uri`. This works correctly.

---

## Test 3: WhereUsed

### Attempt 1: GET (our current implementation)
**Endpoint:** `GET /sap/bc/adt/repository/informationsystem/usageReferences?adtObjectUri=/sap/bc/adt/programs/programs/RSPARAM`
**Status:** 405 Method Not Allowed
**Error:** `Resource controller does not support method GET`

### Attempt 2: POST with correct headers (working)
**Endpoint:** `POST /sap/bc/adt/repository/informationsystem/usageReferences?uri=/sap/bc/adt/programs/programs/RSPARAM`
**Content-Type:** `application/vnd.sap.adt.repository.usagereferences.request.v1+xml`
**Accept:** `application/vnd.sap.adt.repository.usagereferences.result.v1+xml`
**Status:** 200 OK

**Request body required:**
```xml
<?xml version="1.0" encoding="UTF-8"?>
<usageReferences:usageReferenceRequest
  xmlns:usageReferences="http://www.sap.com/adt/ris/usageReferences"
  xmlns:adtcore="http://www.sap.com/adt/core">
  <adtcore:objectReference adtcore:uri="/sap/bc/adt/programs/programs/RSPARAM"/>
</usageReferences:usageReferenceRequest>
```

**Response (excerpt):**
```xml
<?xml version="1.0" encoding="utf-8"?>
<usageReferences:usageReferenceResult numberOfResults="16"
  resultDescription="[S4U] Where-Used List: RSPARAM (Program)"
  xmlns:usageReferences="http://www.sap.com/adt/ris/usageReferences">
  <usageReferences:referencedObjects>
    <usageReferences:referencedObject uri="/sap/bc/adt/functions/groups/afx_gf_generator"
      parentUri="/sap/bc/adt/packages/afx_generator" isResult="false" canHaveChildren="false">
      <usageReferences:adtObject adtcore:responsible="SAP" adtcore:name="AFX_GF_GENERATOR"
        adtcore:type="FUGR/F" xmlns:adtcore="http://www.sap.com/adt/core">
        <adtcore:packageRef adtcore:uri="/sap/bc/adt/packages/afx_generator"
          adtcore:type="DEVC/K" adtcore:name="AFX_GENERATOR"/>
      </usageReferences:adtObject>
    </usageReferences:referencedObject>
    <!-- ... more results ... -->
  </usageReferences:referencedObjects>
</usageReferences:usageReferenceResult>
```

**Match with our code:** NO -- CRITICAL BUG. Our code:
1. Uses GET -- SAP requires POST (405 error)
2. Uses `doRead` -- needs `doMutate` for POST with CSRF token
3. Uses query param `adtObjectUri` -- SAP expects `uri` as query param
4. Sends no request body -- SAP requires an XML body with specific namespace `http://www.sap.com/adt/ris/usageReferences`
5. Uses generic `Accept: application/xml` -- SAP requires `application/vnd.sap.adt.repository.usagereferences.result.v1+xml`
6. Uses generic `Content-Type` -- SAP requires `application/vnd.sap.adt.repository.usagereferences.request.v1+xml`
7. Response XML uses `<usageReferences:referencedObject>` not `<objectReference>` -- our `parseObjectReferences` would fail even if the request succeeded
8. Response has nested structure (`referencedObjects > referencedObject > adtObject`) with different attributes than our parser expects

---

## Test 4: BrowsePackage (CRITICAL)

### Attempt 1: GET
**Endpoint:** `GET /sap/bc/adt/repository/nodestructure?parent_type=DEVC/K&parent_name=$LOCAL`
**Status:** 405 Method Not Allowed
**Error:** `Resource controller does not support method GET`

### Attempt 2: POST with correct Accept
**Endpoint:** `POST /sap/bc/adt/repository/nodestructure?parent_type=DEVC/K&parent_name=STUN`
**Accept:** `application/vnd.sap.as+xml`
**Content-Type:** `application/xml`
**Status:** 200 OK
**Content-Type (response):** `application/vnd.sap.as+xml; charset=utf-8; dataname=com.sap.adt.RepositoryObjectTreeContent`

**Response (excerpt):**
```xml
<?xml version="1.0" encoding="utf-8"?>
<asx:abap version="1.0" xmlns:asx="http://www.sap.com/abapxml">
  <asx:values>
    <DATA>
      <TREE_CONTENT>
        <SEU_ADT_REPOSITORY_OBJ_NODE>
          <OBJECT_TYPE>DEVC/K</OBJECT_TYPE>
          <OBJECT_NAME/>
          <TECH_NAME>STUN</TECH_NAME>
          <OBJECT_URI/>
          <OBJECT_VIT_URI/>
          <EXPANDABLE>X</EXPANDABLE>
          <IS_FINAL/>
          <IS_ABSTRACT/>
          <IS_FOR_TESTING/>
          <IS_EVENT_HANDLER/>
          <IS_CONSTRUCTOR/>
          <IS_REDEFINITION/>
          <IS_STATIC/>
          <IS_READ_ONLY/>
          <IS_CONSTANT/>
          <VISIBILITY>0</VISIBILITY>
          <NODE_ID>000002</NODE_ID>
          <PARENT_NAME/>
          <DESCRIPTION/>
          <DESCRIPTION_TYPE/>
          <VERSION/>
          <INACTIVE_TYPE/>
        </SEU_ADT_REPOSITORY_OBJ_NODE>
        <SEU_ADT_REPOSITORY_OBJ_NODE>
          <OBJECT_TYPE>DEVC/K</OBJECT_TYPE>
          <OBJECT_NAME>STUN_COMMON</OBJECT_NAME>
          <TECH_NAME>STUN_COMMON</TECH_NAME>
          <OBJECT_URI>/sap/bc/adt/packages/stun_common</OBJECT_URI>
          <OBJECT_VIT_URI>/sap/bc/adt/vit/wb/object_type/devck/object_name/STUN_COMMON</OBJECT_VIT_URI>
          <EXPANDABLE>X</EXPANDABLE>
          <!-- ... more fields ... -->
        </SEU_ADT_REPOSITORY_OBJ_NODE>
        <!-- ... more nodes ... -->
      </TREE_CONTENT>
    </DATA>
  </asx:values>
</asx:abap>
```

**Match with our code:** NO -- CRITICAL BUG. Our code:
1. Uses GET (`doRead`) -- SAP requires POST (405 error)
2. Uses `Accept: application/xml` -- SAP requires `application/vnd.sap.as+xml`
3. Expects `<objectReferences>/<objectReference>` XML structure -- SAP returns a completely different format: `<asx:abap>/<asx:values>/<DATA>/<TREE_CONTENT>/<SEU_ADT_REPOSITORY_OBJ_NODE>`
4. Node elements use child elements (`<OBJECT_TYPE>`, `<OBJECT_NAME>`, `<TECH_NAME>`, `<OBJECT_URI>`) instead of attributes
5. There is no `description` or `packageName` in the node -- instead there's `TECH_NAME`, `EXPANDABLE`, `NODE_ID`, `VISIBILITY`, etc.
6. Our `parseObjectReferences()` function would completely fail to parse this response

---

## Test 5: GetObjectInfo

### Attempt 1: Accept: application/xml
**Endpoint:** `GET /sap/bc/adt/programs/programs/RSPARAM`
**Accept:** `application/xml`
**Status:** 406 Not Acceptable
**Error:** `The message content is not acceptable. Accepted content types:`

### Attempt 2: Accept with SAP-specific types (working)
**Endpoint:** `GET /sap/bc/adt/programs/programs/RSPARAM`
**Accept:** `application/vnd.sap.adt.programs.programs.v2+xml, application/vnd.sap.adt.programs.programs+xml, application/xml`
**Status:** 200 OK
**Content-Type:** `application/vnd.sap.adt.programs.programs.v2+xml; charset=utf-8`
**ETag:** `202308031506360018`

**Response (excerpt):**
```xml
<?xml version="1.0" encoding="utf-8"?>
<program:abapProgram
  program:lockedByEditor="false"
  program:programType="executableProgram"
  abapsource:sourceUri="source/main"
  abapsource:fixPointArithmetic="true"
  abapsource:activeUnicodeCheck="true"
  adtcore:responsible="SAP"
  adtcore:masterLanguage="EN"
  adtcore:masterSystem="SAP"
  adtcore:abapLanguageVersion="standard"
  adtcore:name="RSPARAM"
  adtcore:type="PROG/P"
  adtcore:changedAt="2023-08-03T15:06:36Z"
  adtcore:version="active"
  adtcore:changedBy="SAP"
  adtcore:description="Display SAP Profile Parameters"
  adtcore:descriptionTextLimit="70"
  adtcore:language="EN"
  xmlns:program="http://www.sap.com/adt/programs/programs"
  xmlns:abapsource="http://www.sap.com/adt/abapsource"
  xmlns:adtcore="http://www.sap.com/adt/core">
  <atom:link href="source/main" rel="http://www.sap.com/adt/relations/source" type="text/plain"
    etag="202308031506360011" xmlns:atom="http://www.w3.org/2005/Atom"/>
  <adtcore:packageRef adtcore:uri="/sap/bc/adt/packages/stun" adtcore:type="DEVC/K" adtcore:name="STUN"/>
  <!-- ... more elements ... -->
</program:abapProgram>
```

**Match with our code:** NO -- BUG. Our code:
1. Uses `Accept: application/xml` -- SAP returns 406. Need object-type-specific Accept header like `application/vnd.sap.adt.programs.programs.v2+xml`
2. Response is `<program:abapProgram>` not `<objectReference>` -- our `xmlObjectReference` struct would fail to parse
3. The attributes use `adtcore:` prefix (`adtcore:name`, `adtcore:type`, `adtcore:uri` etc.) which is fine for Go's XML parser, but the root element is different
4. The Accept header required varies by object type (programs vs classes vs function groups etc.), making a generic `GetObjectInfo` difficult

---

## Test 6: SyntaxCheck

### Attempt 1: Generic Content-Type and Accept
**Status:** 415 Unsupported Media Type
**Error:** `Supported Media Types: application/vnd.sap.adt.checkobjects+xml`

### Attempt 2: Correct Content-Type, wrong namespace
**Status:** 400 Bad Request
**Error:** `System expected the element '{http://www.sap.com/adt/checkrun}checkObjectList'`

### Attempt 3: Correct Content-Type, Accept, and namespace (working)
**Endpoint:** `POST /sap/bc/adt/checkruns`
**Content-Type:** `application/vnd.sap.adt.checkobjects+xml`
**Accept:** `application/vnd.sap.adt.checkmessages+xml`
**Status:** 200 OK

**Request body:**
```xml
<?xml version="1.0" encoding="UTF-8"?>
<chkrun:checkObjectList xmlns:chkrun="http://www.sap.com/adt/checkrun"
  xmlns:adtcore="http://www.sap.com/adt/core">
  <chkrun:checkObject adtcore:uri="/sap/bc/adt/programs/programs/RSPARAM" chkrun:version="active"/>
</chkrun:checkObjectList>
```

**Response:**
```xml
<?xml version="1.0" encoding="utf-8"?>
<chkrun:checkRunReports xmlns:chkrun="http://www.sap.com/adt/checkrun">
  <chkrun:checkReport chkrun:reporter="abapCheckRun"
    chkrun:triggeringUri="/sap/bc/adt/programs/programs/RSPARAM"
    chkrun:status="processed"
    chkrun:statusText="Object RSPARAM has been checked"/>
</chkrun:checkRunReports>
```

**Match with our code:** NO -- BUG. Our code:
1. Uses `Content-Type: application/xml` -- SAP requires `application/vnd.sap.adt.checkobjects+xml`
2. Uses `Accept: application/xml` -- SAP requires `application/vnd.sap.adt.checkmessages+xml`
3. Sends empty body (`strings.NewReader("")`) -- SAP requires a proper XML body with `<chkrun:checkObjectList>` (namespace `http://www.sap.com/adt/checkrun`)
4. Uses `adtObjectUri` query parameter -- SAP expects the URI in the XML body
5. Response is `<chkrun:checkRunReports>/<chkrun:checkReport>` with attributes, not `<messages>/<message>` -- our `xmlCheckMessages` struct would not parse
6. For a clean program like RSPARAM, there are no individual messages (just a status report)

---

## Test 7: RunUnitTests

**Endpoint:** `POST /sap/bc/adt/abapunit/testruns`
**Content-Type:** `application/xml`
**Accept:** `application/xml`
**Status:** 200 OK

**Request body:**
```xml
<?xml version="1.0" encoding="UTF-8"?>
<aunit:runConfiguration xmlns:aunit="http://www.sap.com/adt/aunit">
  <external><coverage active="false"/></external>
  <options>
    <uriType value="semantic"/>
    <testDeterminationStrategy sameProgram="true" assignedTests="false" publicMethods="false"/>
    <testRiskLevels harmless="true" dangerous="true" critical="true"/>
    <testDurations short="true" medium="true" long="true"/>
  </options>
  <adtcore:objectSets xmlns:adtcore="http://www.sap.com/adt/core">
    <objectSet kind="inclusive">
      <adtcore:objectReferences>
        <adtcore:objectReference adtcore:uri="/sap/bc/adt/programs/programs/RSPARAM"/>
      </adtcore:objectReferences>
    </objectSet>
  </adtcore:objectSets>
</aunit:runConfiguration>
```

**Response (RSPARAM has no tests):**
```xml
<?xml version="1.0" encoding="utf-8"?>
<aunit:runResult xmlns:aunit="http://www.sap.com/adt/aunit"/>
```

**Match with our code:** PARTIAL -- The endpoint path and method work. Would need to verify with an object that actually has unit tests to confirm response parsing. The request body format used here (`runConfiguration` with `objectSets`) is the newer API format. Our code may use a simpler format.

---

## Test 8: GetTransportRequests

### Attempt 1: Accept: application/xml
**Status:** 406 Not Acceptable
**Error:** `Accepted content types: application/vnd.sap.adt.transportorganizertree.v1+xml`

### Attempt 2: Correct Accept header (working)
**Endpoint:** `GET /sap/bc/adt/cts/transportrequests?user=kleink&status=D`
**Accept:** `application/vnd.sap.adt.transportorganizertree.v1+xml`
**Status:** 200 OK
**Content-Type:** `application/vnd.sap.adt.transportorganizertree.v1+xml; charset=utf-8`

**Response (user has no modifiable transport requests):**
```xml
<?xml version="1.0" encoding="utf-8"?>
<tm:root adtcore:name="KLEINK"
  adtcore:changedAt="2026-03-23T21:35:09Z"
  adtcore:createdAt="2026-03-23T21:35:09Z"
  adtcore:changedBy="KLEINK"
  adtcore:createdBy="KLEINK"
  xmlns:tm="http://www.sap.com/cts/adt/tm"
  xmlns:adtcore="http://www.sap.com/adt/core"/>
```

**Match with our code:** NO -- BUG. Our code:
1. Uses `Accept: application/xml` -- SAP requires `application/vnd.sap.adt.transportorganizertree.v1+xml` (406 error)
2. Response root is `<tm:root>` in namespace `http://www.sap.com/cts/adt/tm` -- our `xmlTransportRoot` expects just `<root>` which should match if Go ignores namespace
3. No transport requests exist for this user/status, so we can't verify the inner structure (`workbenchRequests>workbenchRequest`) parsing

---

## Test 9: Activation Endpoint Path

### Path A (our current code): `/sap/bc/adt/activation/activate`
**Endpoint:** `POST /sap/bc/adt/activation/activate?method=activate&preauditRequested=true`
**Status:** 404 Not Found
**Error:** `No suitable resource found`

### Path B (alternative): `/sap/bc/adt/activation`
**Endpoint:** `POST /sap/bc/adt/activation?method=activate&preauditRequested=true`
**Status:** 200 OK
**Content-Type:** `application/xml; charset=utf-8`

**Response:**
```xml
<?xml version="1.0" encoding="utf-8"?>
<chkl:messages xmlns:chkl="http://www.sap.com/abapxml/checklist">
  <chkl:properties checkExecuted="false" activationExecuted="false" generationExecuted="true"/>
</chkl:messages>
```

**Match with our code:** NO -- CRITICAL BUG. Our code:
1. Uses path `/sap/bc/adt/activation/activate` -- SAP returns 404. Correct path is `/sap/bc/adt/activation`
2. Response root is `<chkl:messages>` in namespace `http://www.sap.com/abapxml/checklist` -- our `xmlActivationMessages` expects `<messages>` which should match (Go ignores namespace prefix)
3. Response has `<chkl:properties>` not `<message>` elements for a successful activation -- our parser would return empty messages, which is correct (no errors = success)

---

## Summary of Findings

### Working Correctly (2/9 tested)
| Endpoint | Status |
|----------|--------|
| GetSource | WORKS -- GET, text/plain, ETag all correct |
| SearchObjects | WORKS -- GET, XML parsing matches `objectReferences` format |

### Completely Broken (5/9 tested)
| Endpoint | Issues |
|----------|--------|
| **WhereUsed** | Uses GET (needs POST), wrong query param, wrong Accept/Content-Type, no request body, completely wrong response parser |
| **BrowsePackage** | Uses GET (needs POST), wrong Accept header, completely wrong XML structure expected (`objectReference` vs `SEU_ADT_REPOSITORY_OBJ_NODE`) |
| **ActivateObject** | Wrong URL path (`/activation/activate` vs `/activation`), 404 error |
| **SyntaxCheck** | Wrong Content-Type, wrong Accept, no proper request body, wrong response parser |
| **GetObjectInfo** | Wrong Accept header (needs object-type-specific header), wrong response parser |

### Partially Broken (2/9 tested)
| Endpoint | Issues |
|----------|--------|
| **GetTransportRequests** | Wrong Accept header (needs `application/vnd.sap.adt.transportorganizertree.v1+xml`) |
| **RunUnitTests** | Endpoint path works, but request body format needs verification with real test classes |

### Not Directly Testable
| Endpoint | Reason |
|----------|--------|
| **SetSource** | Would modify system data -- not tested to avoid side effects |
| **AddToTransport** | Requires an existing transport request and object -- not tested |

### Key Bug Categories

1. **Wrong HTTP Method (2 endpoints):** BrowsePackage and WhereUsed use GET but SAP requires POST
2. **Wrong Accept/Content-Type headers (5 endpoints):** SAP uses vendor-specific MIME types, not generic `application/xml`
3. **Wrong URL path (1 endpoint):** Activation uses `/activation/activate` but correct is `/activation`
4. **Wrong XML request body (3 endpoints):** WhereUsed, SyntaxCheck, and BrowsePackage need proper XML request bodies
5. **Wrong response parser (4 endpoints):** WhereUsed, BrowsePackage, SyntaxCheck, and GetObjectInfo expect different XML structures than SAP returns

---

## New Endpoint Verification

**Date:** 2026-03-23
**System:** srvhfuhana.sap.msp.local:44300
**Client:** 100 / User:** kleink

---

### Endpoint 1: Lock Object

**URL:** `POST /sap/bc/adt/programs/programs/{name}?_action=LOCK&accessMode=MODIFY`
**HTTP Method:** POST
**HTTP Status:** 200 OK
**Content-Type (response):** `application/vnd.sap.as+xml; charset=utf-8; dataname=com.sap.adt.lock.Result`
**Required headers:** `X-CSRF-Token: <token>`, `sap-client: <client>`

**Response body (full):**
```xml
<?xml version="1.0" encoding="utf-8"?>
<asx:abap version="1.0" xmlns:asx="http://www.sap.com/abapxml">
  <asx:values>
    <DATA>
      <LOCK_HANDLE>15F0D1EAA10BDCBE10C24098848DC83FF52C7A5F</LOCK_HANDLE>
      <CORRNR/>
      <CORRUSER/>
      <CORRTEXT/>
      <IS_LOCAL/>
      <IS_LINK_UP/>
      <MODIFICATION_SUPPORT>ModificationsLoggedOnly</MODIFICATION_SUPPORT>
      <SCOPE_MESSAGES/>
    </DATA>
  </asx:values>
</asx:abap>
```

**Notes:**
- The `LOCK_HANDLE` is a 40-character hex string needed to unlock
- `CORRNR` / `CORRUSER` / `CORRTEXT` are populated when a transport is assigned
- `IS_LOCAL` is set when object is in a local (`$TMP`) package
- `MODIFICATION_SUPPORT` indicates lock scope

**Conclusion:** AVAILABLE. Response format is `asx:abap` wrapper with `DATA/LOCK_HANDLE`. Needs custom XML parser.

---

### Endpoint 2: Unlock Object

**URL:** `POST /sap/bc/adt/programs/programs/{name}?_action=UNLOCK&lockHandle={handle}`
**HTTP Method:** POST (not DELETE)
**HTTP Status:** 200 OK (empty body on success)
**Required headers:** `X-CSRF-Token: <token>`, `sap-client: <client>`

**Notes:**
- DELETE method on same URL returns 423 with `ExceptionResourceInvalidLockHandle` even with a valid handle
- POST with `_action=UNLOCK` returns 200 empty body on success
- The lock handle is the hex string from the LOCK response

**Conclusion:** AVAILABLE. Use POST (not DELETE) with `_action=UNLOCK` query param.

---

### Endpoint 3: Transport Checks

**URL:** `POST /sap/bc/adt/cts/transportchecks`
**HTTP Method:** POST
**HTTP Status:** 200 OK
**Content-Type (request):** `application/vnd.sap.as+xml; charset=utf-8; dataname=com.sap.adt.transport.CheckObjects`
**Content-Type (response):** `application/vnd.sap.as+xml; charset=utf-8; dataname=com.sap.adt.transport.service.checkData`
**Required headers:** `X-CSRF-Token: <token>`, `sap-client: <client>`

**Request body:**
```xml
<?xml version="1.0" encoding="UTF-8"?>
<asx:abap version="1.0" xmlns:asx="http://www.sap.com/abapxml">
  <asx:values>
    <DATA>
      <PGMID>R3TR</PGMID>
      <OBJECT>PROG</OBJECT>
      <OBJECTNAME>RSPARAM</OBJECTNAME>
      <OPERATION>I</OPERATION>
    </DATA>
  </asx:values>
</asx:abap>
```

**Response body (excerpt):**
```xml
<?xml version="1.0" encoding="utf-8"?>
<asx:abap version="1.0" xmlns:asx="http://www.sap.com/abapxml">
  <asx:values>
    <DATA>
      <PGMID>R3TR</PGMID>
      <OBJECT>PROG</OBJECT>
      <OBJECTNAME>RSPARAM</OBJECTNAME>
      <OPERATION>I</OPERATION>
      <DEVCLASS>STUN</DEVCLASS>
      <CTEXT>SAP Monitoring Tools</CTEXT>
      <KORRFLAG>X</KORRFLAG>
      <AS4USER>SAP</AS4USER>
      <PDEVCLASS>SAP</PDEVCLASS>
      <DLVUNIT>SAP_BASIS</DLVUNIT>
      <NAMESPACE>/0SAP/</NAMESPACE>
      <RESULT>S</RESULT>
      <RECORDING>X</RECORDING>
      <MESSAGES>
        <CTS_MESSAGE>
          <SEVERITY>S</SEVERITY>
          <ARBGB>TR</ARBGB>
          <MSGNR>015</MSGNR>
          <TEXT>Object can only be created in SAP package</TEXT>
        </CTS_MESSAGE>
      </MESSAGES>
      <REQUESTS>
        <CTS_REQUEST>
          <REQ_HEADER>
            <TRKORR>S4UK902321</TRKORR>
            <TRFUNCTION>K</TRFUNCTION>
            <TRSTATUS>D</TRSTATUS>
            <TARSYSTEM>DUM</TARSYSTEM>
            <AS4USER>KLEINK</AS4USER>
            <AS4DATE>2026-03-20</AS4DATE>
            <AS4TEXT>zdm_sql</AS4TEXT>
          </REQ_HEADER>
        </CTS_REQUEST>
        <!-- ... more requests ... -->
      </REQUESTS>
    </DATA>
  </asx:values>
</asx:abap>
```

**Notes:**
- `RESULT` field: `S` = success/recordable, `E` = error (e.g., invalid object)
- `REQUESTS` contains a list of existing open transport requests the user can choose from
- `RECORDING` = `X` means the object can be recorded into a transport
- Without proper body, returns 400 `No data type found in content type ''`
- With wrong Accept, returns 406 listing required types

**Conclusion:** AVAILABLE. Uses `asx:abap` format for both request and response. Must parse `DATA/RESULT` and `DATA/REQUESTS/CTS_REQUEST/REQ_HEADER` for transport list.

---

### Endpoint 4: Create Transport

**URL:** `POST /sap/bc/adt/cts/transports`
**HTTP Method:** POST
**HTTP Status:** 200 OK (empty body — transport number not returned in response headers or body)
**Content-Type (request):** `application/vnd.sap.as+xml; charset=utf-8; dataname=com.sap.adt.transport.WorkbenchTransport`
**Content-Type (response):** none (empty body, content-length: 0)
**Required headers:** `X-CSRF-Token: <token>`, `sap-client: <client>`

**Request body:**
```xml
<?xml version="1.0" encoding="UTF-8"?>
<asx:abap version="1.0" xmlns:asx="http://www.sap.com/abapxml">
  <asx:values>
    <DATA>
      <CATEGORY>K</CATEGORY>
      <TARGET>DUM</TARGET>
      <DESCRIPTION>My transport description</DESCRIPTION>
      <DEVCLASS>$TMP</DEVCLASS>
    </DATA>
  </asx:values>
</asx:abap>
```

**Notes:**
- `CATEGORY`: `K` = Workbench, `W` = Customizing
- `TARGET`: transport target system (e.g., `DUM`, `S4UCLNT100`)
- `DEVCLASS`: must be a valid package that exists (returns 500 with `ExceptionResourceCreationFailure` for non-existent package)
- Response body is empty (content-length: 0); no `Location` header with new transport number
- This is a concern — the transport number is needed for subsequent operations (adding objects)
- Needs further investigation to determine how the created transport number is retrieved

**Conclusion:** AVAILABLE but NEEDS FURTHER INVESTIGATION. Endpoint accepts requests but returns no transport number in the response. Must probe discovery or alternative paths for how the new transport ID is communicated back.

---

### Endpoint 5: ATC Customizing

**URL:** `GET /sap/bc/adt/atc/customizing`
**HTTP Method:** GET
**HTTP Status:** 200 OK
**Content-Type (response):** `application/vnd.sap.atc.customizing-v1+xml; charset=utf-8`
**Required headers:** `sap-client: <client>`

**Response body (full):**
```xml
<?xml version="1.0" encoding="utf-8"?>
<atc:customizing xmlns:atc="http://www.sap.com/adt/atc">
  <properties>
    <property name="ciCheckFlavour" value="true"/>
    <property name="systemCheckVariant" value="ZCB_CLEAN_ABAP_1"/>
    <property name="isCCSTunnelEnabled" value="false"/>
    <property name="isTransportableExemptionTypeUsed" value="false"/>
  </properties>
  <exemption>
    <reasons>
      <reason id="FPOS" justificationMandatory="true" title="False Positive - finding does not apply - see justification"/>
      <reason id="OTHR" justificationMandatory="true" title="Other Reason - see justification"/>
    </reasons>
    <validities>
      <validity id="U" value="No Restrictions"/>
      <validity id="D" value="Date"/>
    </validities>
  </exemption>
  <scaAttributes>
    <scaAttribute labelL="Additional Info" labelM="Additional Info" labelS="Add. Info" label="false" attributeName="ADD_INFO"/>
    <!-- ... more scaAttribute elements ... -->
  </scaAttributes>
</atc:customizing>
```

**Conclusion:** AVAILABLE. Simple GET, no auth headers beyond basic. Returns `atc:customizing` root with `properties`, `exemption`, `scaAttributes` children.

---

### Endpoint 6: Pretty Printer Settings

**URL:** `GET /sap/bc/adt/abapsource/prettyprinter/settings`
**HTTP Method:** GET
**HTTP Status:** 200 OK
**Content-Type (response):** `application/vnd.sap.adt.ppsettings.v5+xml; charset=utf-8`
**Required headers:** `sap-client: <client>`

**Response body (full):**
```xml
<?xml version="1.0" encoding="utf-8"?>
<abapformatter:PrettyPrinterSettings
  abapformatter:indentation="true"
  abapformatter:style="keywordUpper"
  abapformatter:keepIdentifier="true"
  xmlns:abapformatter="http://www.sap.com/adt/prettyprintersettings"/>
```

**Notes:**
- Single self-closing root element with all settings as attributes
- `style`: `keywordUpper` = keywords in uppercase; other values: `keywordLower`, `keywordPretty`
- `indentation`: boolean, whether to auto-indent
- `keepIdentifier`: boolean, whether to preserve identifier case

**Conclusion:** AVAILABLE. Simple GET, minimal response. Single XML element with attributes only.

---

### Endpoint 7: Code Completion

**URL:** `POST /sap/bc/adt/abapsource/codecompletion/proposal`
**HTTP Method:** POST
**HTTP Status:** 200 OK (empty body when no completions available)
**Content-Type (request):** `text/plain`
**Accept (request):** `application/vnd.sap.as+xml`
**Content-Type (response):** none listed (content-length: 0 when empty)
**Required headers:** `X-CSRF-Token: <token>`, `sap-client: <client>`

**Query parameters:**
- `uri`: ADT source URI (e.g., `/sap/bc/adt/programs/programs/RSPARAM/source/main`)
- `row`: 1-based row number in the source
- `col`: 1-based column number

**Request body:** The current source text (plain text, not XML)

**Notes:**
- Without `uri` parameter: 400 `Parameter uri could not be found`
- Without correct Accept header: 406, accepted types: `application/vnd.sap.as+xml`
- Attempting to use a fragment (`#row,col`) in the URI causes 400 `URI-Mapping cannot be performed due to invalid URI`
- Use `row` and `col` query params instead of URI fragment
- An empty response (content-length: 0) means no completions available for the given position/source
- Could not provoke a non-empty completion response in testing (RSPARAM is a read-only SAP object)

**Conclusion:** AVAILABLE. Endpoint accepts requests and returns 200. Response format when completions exist needs further investigation with a custom (non-SAP-delivered) ABAP program.

---

### Endpoint 8: Navigation Target

**URL:** `POST /sap/bc/adt/navigation/target`
**HTTP Method:** POST (GET returns 405)
**HTTP Status:** 400 Bad Request (with incomplete/wrong URI)
**Content-Type (response):** `application/xml` (error responses)
**Required headers:** `X-CSRF-Token: <token>`, `sap-client: <client>`

**Query parameters:**
- `uri`: ADT object or source URI

**Notes:**
- GET method: 405 `Resource controller does not support method GET`
- POST with `uri=/sap/bc/adt/programs/programs/RSPARAM`: 400 `NavigationFailure` with message `I::000` (empty message key — no navigation target found for program object itself)
- POST with source reference `uri=.../source/main&line=N&column=M`: same 400 NavigationFailure
- POST with URI fragment (`#start...`): 400 `URI-Mapping cannot be performed due to invalid URI`
- The endpoint EXISTS and rejects navigating to a program (it expects a usage/reference URI, not the object itself)
- Likely requires a URI pointing to a specific element within source code (e.g., a class call site) to return a navigation target

**Conclusion:** AVAILABLE but NEEDS FURTHER INVESTIGATION for exact request format. The endpoint exists and is functional (rejects invalid navigations properly), but could not provoke a success response with the test objects available.

---

### Endpoint 9: Inactive Objects

**URL:** `GET /sap/bc/adt/activation/inactiveobjects`
**HTTP Method:** GET
**HTTP Status:** 200 OK
**Content-Type (response):** `application/vnd.sap.adt.inactivectsobjects.v1+xml; charset=utf-8`
**Required headers:** `sap-client: <client>`

**Response body (full, when no inactive objects exist):**
```xml
<?xml version="1.0" encoding="utf-8"?>
<ioc:inactiveObjects xmlns:ioc="http://www.sap.com/abapxml/inactiveCtsObjects"/>
```

**Notes:**
- Returns an empty root element when no inactive objects exist for the current user
- When inactive objects are present, would contain child elements
- No CSRF token required (read-only GET)

**Conclusion:** AVAILABLE. Simple GET. Empty `ioc:inactiveObjects` root when nothing inactive. Full schema for child elements needs to be determined from activation workflow testing.

---

### Endpoint 10: ABAP Keyword Documentation

**URL:** `GET /sap/bc/adt/docu/abap/langu` (also accepts POST — returns same HTML)
**HTTP Method:** GET preferred (POST also works)
**HTTP Status:** 200 OK
**Content-Type (response):** `application/vnd.sap.adt.docu.v1+html; charset=utf-8`
**Required headers:** `sap-client: <client>`

**Query parameters:**
- `objectUri`: ADT object URI (optional — context for the documentation)
- `keyword`: specific ABAP keyword to look up (optional)
- `context`: context type, e.g., `ABAP_KEYWORD`

**Response body:** Full HTML page with ABAP keyword documentation (SAP Fiori-styled). The response is several hundred KB of HTML.

**Notes:**
- GET with no parameters returns the ABAP keyword documentation landing page (200 OK, not an error)
- GET with `?objectUri=...` returns object-relevant documentation
- GET with `?keyword=REPORT&context=ABAP_KEYWORD` returns documentation for the `REPORT` keyword
- POST with no body also returns 200 with HTML (the ABAP docs index page)
- Response is HTML, not XML — cannot be parsed with standard XML parsers

**Conclusion:** AVAILABLE. Returns HTML documentation. Can be surfaced to AI agents as-is (HTML content) or via a text extraction wrapper.

---

### Endpoint 11: Logoff

**URL:** `GET /sap/public/bc/icf/logoff`
**HTTP Method:** GET
**HTTP Status:** 200 OK
**Content-Type (response):** `text/html; charset=utf-8`
**Required headers:** `sap-client: <client>`

**Response body:** Standard SAP "Goodbye — You have been logged off" HTML page.

**Notes:**
- This endpoint invalidates the current session cookie
- After calling this, the CSRF token and session cookie are no longer valid
- Must re-authenticate after calling this endpoint

**Conclusion:** AVAILABLE. Simple GET. Returns HTML goodbye page and invalidates session.

---

## Summary: New Endpoint Verification

| # | Endpoint | Method | Status | Conclusion |
|---|----------|--------|--------|------------|
| 1 | `/sap/bc/adt/programs/programs/{name}?_action=LOCK` | POST | 200 | AVAILABLE |
| 2 | `/sap/bc/adt/programs/programs/{name}?_action=UNLOCK` | POST | 200 | AVAILABLE |
| 3 | `/sap/bc/adt/cts/transportchecks` | POST | 200 | AVAILABLE |
| 4 | `/sap/bc/adt/cts/transports` (create) | POST | 200 | AVAILABLE (transport number in response unknown) |
| 5 | `/sap/bc/adt/atc/customizing` | GET | 200 | AVAILABLE |
| 6 | `/sap/bc/adt/abapsource/prettyprinter/settings` | GET | 200 | AVAILABLE |
| 7 | `/sap/bc/adt/abapsource/codecompletion/proposal` | POST | 200 | AVAILABLE (empty response format needs further testing) |
| 8 | `/sap/bc/adt/navigation/target` | POST | 400* | AVAILABLE (correct format not yet determined) |
| 9 | `/sap/bc/adt/activation/inactiveobjects` | GET | 200 | AVAILABLE |
| 10 | `/sap/bc/adt/docu/abap/langu` | GET | 200 | AVAILABLE |
| 11 | `/sap/public/bc/icf/logoff` | GET | 200 | AVAILABLE |

*400 is a validation error (not 404), confirming the endpoint exists.

### Key Findings

1. **All 11 new endpoints exist** — none returned 404.
2. **Lock/Unlock pattern**: LOCK uses POST with `_action=LOCK`, UNLOCK also uses POST (not DELETE) with `_action=UNLOCK`. Response is `asx:abap` wrapper format.
3. **Transport checks** use `asx:abap` XML format for both request and response (not JSON or generic XML). Request body must include `PGMID`, `OBJECT`, `OBJECTNAME`, `OPERATION` fields.
4. **Create transport** accepts the correct request but returns no transport number in the response (content-length: 0). This needs further investigation — the transport number may need to be retrieved separately (e.g., via `GetTransportRequests`).
5. **ATC customizing, pretty printer, inactive objects** are simple GETs requiring no CSRF token and returning vendor-specific XML.
6. **Code completion** requires `row`/`col` query parameters (not URI fragments), `Content-Type: text/plain` for the source body, and `Accept: application/vnd.sap.as+xml`.
7. **Navigation target** POST exists but the exact URI format that produces a successful navigation could not be determined — `I::000` error suggests the test object URI does not have navigable sub-elements.
8. **ABAP docs** returns HTML content (`application/vnd.sap.adt.docu.v1+html`), not XML.
9. **Logoff** invalidates the session — must be the last call before re-authentication.
