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
