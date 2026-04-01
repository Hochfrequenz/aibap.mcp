CLASS zcl_abapgit_adt_imp_app DEFINITION
  PUBLIC
  INHERITING FROM cl_adt_disc_res_app_base
  FINAL
  CREATE PUBLIC.

  PUBLIC SECTION.

    METHODS if_adt_rest_rfc_application~get_static_uri_path REDEFINITION.

  PROTECTED SECTION.

    METHODS get_application_title REDEFINITION.
    METHODS register_resources REDEFINITION.

  PRIVATE SECTION.

ENDCLASS.



CLASS zcl_abapgit_adt_imp_app IMPLEMENTATION.

  METHOD get_application_title.
    result = 'abapGit Package Import'.
  ENDMETHOD.


  METHOD if_adt_rest_rfc_application~get_static_uri_path.
    result = '/sap/bc/adt/abapgit/import'.
  ENDMETHOD.


  METHOD register_resources.
    registry->register_discoverable_resource(
      url             = '/packages'
      handler_class   = 'ZCL_ABAPGIT_ADT_IMP_RES'
      description     = 'Import abapGit Package from ZIP'
      category_scheme = 'http://www.sap.com/adt/categories/abapgit'
      category_term   = 'import' ).
  ENDMETHOD.

ENDCLASS.
