CLASS zcl_abapgit_adt_imp_res DEFINITION
  PUBLIC
  INHERITING FROM cl_adt_rest_resource
  FINAL
  CREATE PUBLIC.

  PUBLIC SECTION.

    METHODS post REDEFINITION.

  PROTECTED SECTION.
  PRIVATE SECTION.

    METHODS get_package_name
      IMPORTING
        io_request        TYPE REF TO if_adt_rest_request
      RETURNING
        VALUE(rv_package) TYPE devclass.

    METHODS get_transport
      IMPORTING
        io_request          TYPE REF TO if_adt_rest_request
      RETURNING
        VALUE(rv_transport) TYPE trkorr.

ENDCLASS.



CLASS zcl_abapgit_adt_imp_res IMPLEMENTATION.

  METHOD post.

    DATA lv_zip          TYPE xstring.
    DATA lv_folder_logic TYPE string.
    DATA lt_files        TYPE zif_abapgit_git_definitions=>ty_files_tt.
    DATA lo_dot_abapgit  TYPE REF TO zcl_abapgit_dot_abapgit.
    DATA lo_repo         TYPE REF TO zcl_abapgit_repo_offline.
    DATA ls_checks       TYPE zif_abapgit_definitions=>ty_deserialize_checks.
    DATA li_log          TYPE REF TO zif_abapgit_log.
    DATA lv_count        TYPE i.

    " Read and validate package parameter
    DATA(lv_package) = get_package_name( request ).

    IF lv_package IS INITIAL.
      response->set_status( cl_rest_status_code=>gc_client_error_bad_request ).
      RETURN.
    ENDIF.

    " Read transport parameter
    DATA(lv_transport) = get_transport( request ).

    " Read optional folder logic
    TRY.
        request->get_uri_query_parameter(
          EXPORTING name = 'folderLogic'
          IMPORTING value = lv_folder_logic ).
      CATCH cx_adt_rest.
        lv_folder_logic = 'PREFIX'.
    ENDTRY.
    IF lv_folder_logic IS INITIAL.
      lv_folder_logic = 'PREFIX'.
    ENDIF.

    " Read ZIP binary from request body
    DATA(lo_inner_request) = request->get_inner_rest_request( ).
    DATA(lo_entity) = lo_inner_request->get_entity( ).
    lv_zip = lo_entity->get_binary_data( ).

    IF lv_zip IS INITIAL.
      response->set_status( cl_rest_status_code=>gc_client_error_bad_request ).
      DATA(lo_err_response) = response->get_inner_rest_response( ).
      DATA(lo_err_entity) = lo_err_response->create_entity( ).
      lo_err_entity->set_content_type( iv_media_type = 'text/plain' ).
      lo_err_entity->set_string_data( 'No ZIP data in request body' ).
      RETURN.
    ENDIF.

    TRY.
        " Parse ZIP into file list
        lt_files = zcl_abapgit_zip=>load( lv_zip ).

        " Build dot_abapgit configuration
        lo_dot_abapgit = zcl_abapgit_dot_abapgit=>build_default( ).
        lo_dot_abapgit->set_folder_logic( lv_folder_logic ).

        " Create offline repo for deserialization
        DATA(li_repo) = zcl_abapgit_repo_srv=>get_instance( )->new_offline(
          iv_name         = |import_{ lv_package }|
          iv_package      = lv_package
          iv_folder_logic = lv_folder_logic ).

        lo_repo ?= li_repo.

        " Set the files on the repo
        lo_repo->set_files_remote( lt_files ).

        " Run deserialization checks
        ls_checks = zcl_abapgit_objects=>deserialize_checks( lo_repo ).

        " Auto-accept checks: set transport if provided
        IF lv_transport IS NOT INITIAL.
          LOOP AT ls_checks-overwrite ASSIGNING FIELD-SYMBOL(<ls_overwrite>).
            <ls_overwrite>-decision = 'Y'.
          ENDLOOP.
          ls_checks-transport-transport = lv_transport.
        ENDIF.

        " Create log
        CREATE OBJECT li_log TYPE zcl_abapgit_log.

        " Deserialize (import)
        zcl_abapgit_objects=>deserialize(
          io_repo   = lo_repo
          is_checks = ls_checks
          ii_log    = li_log ).

        lv_count = li_log->count( ).

        " Clean up: remove the temporary offline repo
        zcl_abapgit_repo_srv=>get_instance( )->delete( lo_repo ).

      CATCH cx_root INTO DATA(lx_error).
        " Clean up repo on error
        IF lo_repo IS BOUND.
          TRY.
              zcl_abapgit_repo_srv=>get_instance( )->delete( lo_repo ).
            CATCH cx_root ##NO_HANDLER.
          ENDTRY.
        ENDIF.
        response->set_status( cl_rest_status_code=>gc_server_error_internal ).
        lo_err_response = response->get_inner_rest_response( ).
        lo_err_entity = lo_err_response->create_entity( ).
        lo_err_entity->set_content_type( iv_media_type = 'text/plain' ).
        lo_err_entity->set_string_data( lx_error->get_text( ) ).
        RETURN.
    ENDTRY.

    " Success response
    DATA(lo_ok_response) = response->get_inner_rest_response( ).
    DATA(lo_ok_entity) = lo_ok_response->create_entity( ).
    lo_ok_entity->set_content_type( iv_media_type = 'text/plain' ).
    lo_ok_entity->set_string_data( |Import successful. { lv_count } log entries.| ).
    response->set_status( cl_rest_status_code=>gc_success_ok ).

  ENDMETHOD.


  METHOD get_package_name.

    DATA lv_package TYPE string.

    TRY.
        io_request->get_uri_query_parameter(
          EXPORTING name  = 'package'
          IMPORTING value = lv_package ).
      CATCH cx_adt_rest.
        RETURN.
    ENDTRY.

    rv_package = to_upper( lv_package ).

  ENDMETHOD.


  METHOD get_transport.

    DATA lv_transport TYPE string.

    TRY.
        io_request->get_uri_query_parameter(
          EXPORTING name  = 'transport'
          IMPORTING value = lv_transport ).
      CATCH cx_adt_rest.
        RETURN.
    ENDTRY.

    rv_transport = to_upper( lv_transport ).

  ENDMETHOD.

ENDCLASS.
