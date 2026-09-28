-- Schema of the Rails application's database (db/schema.rb), dumped by
-- tools/schema-dump.sh. The Rails migrations own the schema; this snapshot
-- only creates new local databases for the Go server, its tests and its seeder.

--
-- PostgreSQL database dump
--



SET statement_timeout = 0;
SET lock_timeout = 0;
SET idle_in_transaction_session_timeout = 0;
SET client_encoding = 'UTF8';
SET standard_conforming_strings = on;
SELECT pg_catalog.set_config('search_path', '', false);
SET check_function_bodies = false;
SET xmloption = content;
SET client_min_messages = warning;
SET row_security = off;

--
-- Name: amcheck; Type: EXTENSION; Schema: -; Owner: -
--

CREATE EXTENSION IF NOT EXISTS amcheck WITH SCHEMA public;


--
-- Name: pageinspect; Type: EXTENSION; Schema: -; Owner: -
--

CREATE EXTENSION IF NOT EXISTS pageinspect WITH SCHEMA public;


SET default_tablespace = '';

SET default_table_access_method = heap;

--
-- Name: active_storage_attachments; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.active_storage_attachments (
    id bigint NOT NULL,
    blob_id bigint NOT NULL,
    created_at timestamp(6) without time zone NOT NULL,
    name character varying NOT NULL,
    record_id bigint NOT NULL,
    record_type character varying NOT NULL
);


--
-- Name: active_storage_attachments_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.active_storage_attachments_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: active_storage_attachments_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.active_storage_attachments_id_seq OWNED BY public.active_storage_attachments.id;


--
-- Name: active_storage_blobs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.active_storage_blobs (
    id bigint NOT NULL,
    byte_size bigint NOT NULL,
    checksum character varying,
    content_type character varying,
    created_at timestamp(6) without time zone NOT NULL,
    filename character varying NOT NULL,
    key character varying NOT NULL,
    metadata text,
    service_name character varying NOT NULL
);


--
-- Name: active_storage_blobs_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.active_storage_blobs_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: active_storage_blobs_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.active_storage_blobs_id_seq OWNED BY public.active_storage_blobs.id;


--
-- Name: active_storage_variant_records; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.active_storage_variant_records (
    id bigint NOT NULL,
    blob_id bigint NOT NULL,
    variation_digest character varying NOT NULL
);


--
-- Name: active_storage_variant_records_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.active_storage_variant_records_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: active_storage_variant_records_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.active_storage_variant_records_id_seq OWNED BY public.active_storage_variant_records.id;


--
-- Name: api_key_usage_flushes; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.api_key_usage_flushes (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    created_at timestamp(6) without time zone NOT NULL
);


--
-- Name: api_key_usage_logs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.api_key_usage_logs (
    id bigint NOT NULL,
    api_key_id bigint NOT NULL,
    created_at timestamp(6) without time zone NOT NULL,
    date date NOT NULL,
    endpoint character varying NOT NULL,
    requests_count integer DEFAULT 0 NOT NULL,
    updated_at timestamp(6) without time zone NOT NULL
);


--
-- Name: api_key_usage_logs_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.api_key_usage_logs_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: api_key_usage_logs_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.api_key_usage_logs_id_seq OWNED BY public.api_key_usage_logs.id;


--
-- Name: api_keys; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.api_keys (
    id bigint NOT NULL,
    active boolean DEFAULT true NOT NULL,
    billing_active boolean DEFAULT true NOT NULL,
    contact_email character varying NOT NULL,
    created_at timestamp(6) without time zone NOT NULL,
    developer_id bigint,
    expires_at timestamp(6) without time zone,
    key character varying NOT NULL,
    last_used_at timestamp(6) without time zone,
    name character varying NOT NULL,
    rate_limit_multiplier integer DEFAULT 1 NOT NULL,
    requests_count integer DEFAULT 0 NOT NULL,
    updated_at timestamp(6) without time zone NOT NULL
);


--
-- Name: api_keys_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.api_keys_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: api_keys_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.api_keys_id_seq OWNED BY public.api_keys.id;


--
-- Name: ar_internal_metadata; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.ar_internal_metadata (
    key character varying NOT NULL,
    value character varying,
    created_at timestamp(6) without time zone NOT NULL,
    updated_at timestamp(6) without time zone NOT NULL
);


--
-- Name: audio_clip_candidates; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.audio_clip_candidates (
    id bigint NOT NULL,
    audio_clip_id bigint NOT NULL,
    audio_operation_id bigint,
    character_count integer DEFAULT 0 NOT NULL,
    configuration_fingerprint character varying,
    created_at timestamp(6) without time zone NOT NULL,
    custom_instructions text,
    custom_instructions_sha256 character varying,
    duration double precision,
    error_message text,
    filename character varying NOT NULL,
    instructions_sha256 character varying,
    key character varying NOT NULL,
    language character varying NOT NULL,
    line_type character varying DEFAULT ''::character varying NOT NULL,
    model character varying NOT NULL,
    provider character varying NOT NULL,
    speed double precision DEFAULT 1.0 NOT NULL,
    status character varying DEFAULT 'pending'::character varying NOT NULL,
    text text NOT NULL,
    updated_at timestamp(6) without time zone NOT NULL,
    voice character varying NOT NULL
);


--
-- Name: audio_clip_candidates_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.audio_clip_candidates_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: audio_clip_candidates_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.audio_clip_candidates_id_seq OWNED BY public.audio_clip_candidates.id;


--
-- Name: audio_clip_customizations; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.audio_clip_customizations (
    id bigint NOT NULL,
    created_at timestamp(6) without time zone NOT NULL,
    created_by character varying,
    instructions text DEFAULT ''::text NOT NULL,
    instructions_sha256 character varying NOT NULL,
    language character varying NOT NULL,
    model character varying NOT NULL,
    provider character varying NOT NULL,
    status character varying DEFAULT 'active'::character varying NOT NULL,
    text text NOT NULL,
    text_digest character varying NOT NULL,
    updated_at timestamp(6) without time zone NOT NULL,
    voice character varying NOT NULL
);


--
-- Name: audio_clip_customizations_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.audio_clip_customizations_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: audio_clip_customizations_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.audio_clip_customizations_id_seq OWNED BY public.audio_clip_customizations.id;


--
-- Name: audio_clip_usages; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.audio_clip_usages (
    id bigint NOT NULL,
    audio_clip_id bigint NOT NULL,
    created_at timestamp(6) without time zone NOT NULL,
    prayer_book_code character varying NOT NULL,
    source_key character varying DEFAULT ''::character varying NOT NULL,
    source_name character varying NOT NULL,
    updated_at timestamp(6) without time zone NOT NULL
);


--
-- Name: audio_clip_usages_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.audio_clip_usages_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: audio_clip_usages_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.audio_clip_usages_id_seq OWNED BY public.audio_clip_usages.id;


--
-- Name: audio_clips; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.audio_clips (
    id bigint NOT NULL,
    character_count integer DEFAULT 0 NOT NULL,
    configuration_fingerprint character varying,
    created_at timestamp(6) without time zone NOT NULL,
    custom_instructions_sha256 character varying,
    duration double precision,
    filename character varying NOT NULL,
    instructions_sha256 character varying,
    key character varying NOT NULL,
    kind character varying DEFAULT 'line'::character varying NOT NULL,
    language character varying DEFAULT 'pt-BR'::character varying NOT NULL,
    line_type character varying DEFAULT ''::character varying NOT NULL,
    model character varying NOT NULL,
    provider character varying NOT NULL,
    speed double precision DEFAULT 1.0 NOT NULL,
    text text DEFAULT ''::text NOT NULL,
    updated_at timestamp(6) without time zone NOT NULL,
    voice character varying NOT NULL
);


--
-- Name: audio_clips_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.audio_clips_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: audio_clips_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.audio_clips_id_seq OWNED BY public.audio_clips.id;


--
-- Name: audio_generation_sessions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.audio_generation_sessions (
    id bigint NOT NULL,
    completed_at timestamp(6) without time zone,
    created_at timestamp(6) without time zone NOT NULL,
    current_text_id bigint,
    current_voice_key character varying,
    error_log text,
    failed_count integer DEFAULT 0,
    prayer_book_code character varying NOT NULL,
    processed_count integer DEFAULT 0,
    started_at timestamp(6) without time zone,
    status character varying DEFAULT 'running'::character varying NOT NULL,
    total_texts integer DEFAULT 0,
    updated_at timestamp(6) without time zone NOT NULL,
    voice_keys character varying[] DEFAULT '{}'::character varying[],
    CONSTRAINT audio_generation_sessions_state_valid CHECK ((((status)::text = ANY (ARRAY[('running'::character varying)::text, ('completed'::character varying)::text, ('failed'::character varying)::text, ('cancelled'::character varying)::text])) AND (total_texts >= 0) AND (processed_count >= 0) AND (failed_count >= 0)))
);


--
-- Name: audio_generation_sessions_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.audio_generation_sessions_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: audio_generation_sessions_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.audio_generation_sessions_id_seq OWNED BY public.audio_generation_sessions.id;


--
-- Name: audio_operations; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.audio_operations (
    id bigint NOT NULL,
    active_job_id character varying,
    completed_at timestamp(6) without time zone,
    created_at timestamp(6) without time zone NOT NULL,
    error_message text,
    failed_items integer DEFAULT 0 NOT NULL,
    generated_characters integer DEFAULT 0 NOT NULL,
    generated_clips integer DEFAULT 0 NOT NULL,
    kind character varying NOT NULL,
    parameters jsonb DEFAULT '{}'::jsonb NOT NULL,
    prayer_book_code character varying,
    processed_items integer DEFAULT 0 NOT NULL,
    requested_by character varying,
    result jsonb DEFAULT '{}'::jsonb NOT NULL,
    skipped_clips integer DEFAULT 0 NOT NULL,
    started_at timestamp(6) without time zone,
    status character varying DEFAULT 'queued'::character varying NOT NULL,
    total_items integer DEFAULT 0 NOT NULL,
    updated_at timestamp(6) without time zone NOT NULL
);


--
-- Name: audio_operations_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.audio_operations_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: audio_operations_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.audio_operations_id_seq OWNED BY public.audio_operations.id;


--
-- Name: background_categories; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.background_categories (
    id bigint NOT NULL,
    created_at timestamp(6) without time zone NOT NULL,
    name character varying NOT NULL,
    "position" integer DEFAULT 0 NOT NULL,
    slug character varying NOT NULL,
    updated_at timestamp(6) without time zone NOT NULL
);


--
-- Name: background_categories_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.background_categories_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: background_categories_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.background_categories_id_seq OWNED BY public.background_categories.id;


--
-- Name: background_track_assets; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.background_track_assets (
    id bigint NOT NULL,
    active boolean DEFAULT false NOT NULL,
    bytes bigint NOT NULL,
    created_at timestamp(6) without time zone NOT NULL,
    duration_ms bigint NOT NULL,
    loop_end_ms bigint,
    loop_start_ms bigint,
    lufs numeric(6,2),
    mime_type character varying NOT NULL,
    object_key character varying NOT NULL,
    profile character varying NOT NULL,
    sha256 character varying NOT NULL,
    status character varying DEFAULT 'uploaded'::character varying NOT NULL,
    track_id bigint NOT NULL,
    updated_at timestamp(6) without time zone NOT NULL,
    CONSTRAINT background_track_assets_bytes_positive CHECK ((bytes > 0)),
    CONSTRAINT background_track_assets_duration_positive CHECK ((duration_ms > 0)),
    CONSTRAINT background_track_assets_status_valid CHECK (((status)::text = ANY (ARRAY[('uploaded'::character varying)::text, ('ready'::character varying)::text, ('withdrawn'::character varying)::text, ('rejected'::character varying)::text])))
);


--
-- Name: background_track_assets_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.background_track_assets_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: background_track_assets_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.background_track_assets_id_seq OWNED BY public.background_track_assets.id;


--
-- Name: background_track_categories; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.background_track_categories (
    id bigint NOT NULL,
    category_id bigint NOT NULL,
    created_at timestamp(6) without time zone NOT NULL,
    track_id bigint NOT NULL,
    updated_at timestamp(6) without time zone NOT NULL
);


--
-- Name: background_track_categories_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.background_track_categories_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: background_track_categories_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.background_track_categories_id_seq OWNED BY public.background_track_categories.id;


--
-- Name: background_track_placements; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.background_track_placements (
    id bigint NOT NULL,
    created_at timestamp(6) without time zone NOT NULL,
    facet character varying NOT NULL,
    key character varying NOT NULL,
    prayer_book_code character varying,
    priority integer DEFAULT 100 NOT NULL,
    track_id bigint NOT NULL,
    updated_at timestamp(6) without time zone NOT NULL,
    CONSTRAINT background_track_placements_facet_valid CHECK (((facet)::text = ANY (ARRAY[('celebration'::character varying)::text, ('book_season'::character varying)::text, ('season'::character varying)::text, ('office'::character varying)::text, ('default'::character varying)::text])))
);


--
-- Name: background_track_placements_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.background_track_placements_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: background_track_placements_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.background_track_placements_id_seq OWNED BY public.background_track_placements.id;


--
-- Name: background_tracks; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.background_tracks (
    id bigint NOT NULL,
    background_eligible boolean DEFAULT false NOT NULL,
    composer character varying,
    created_at timestamp(6) without time zone NOT NULL,
    credit_text text,
    duration_ms bigint NOT NULL,
    kind character varying NOT NULL,
    license_type character varying,
    license_url character varying,
    license_version character varying,
    loop_approved boolean DEFAULT false NOT NULL,
    notes text,
    performer character varying,
    published_at timestamp(6) without time zone,
    recording_rights_holder character varying,
    slug character varying NOT NULL,
    sort_order integer DEFAULT 0 NOT NULL,
    source_reference character varying,
    source_revision character varying,
    source_url character varying,
    status character varying DEFAULT 'ready'::character varying NOT NULL,
    title character varying NOT NULL,
    updated_at timestamp(6) without time zone NOT NULL,
    CONSTRAINT background_tracks_duration_positive CHECK ((duration_ms > 0)),
    CONSTRAINT background_tracks_kind_valid CHECK (((kind)::text = ANY (ARRAY[('instrumental'::character varying)::text, ('chant'::character varying)::text, ('choral'::character varying)::text]))),
    CONSTRAINT background_tracks_status_valid CHECK (((status)::text = ANY (ARRAY[('ready'::character varying)::text, ('withdrawn'::character varying)::text])))
);


--
-- Name: background_tracks_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.background_tracks_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: background_tracks_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.background_tracks_id_seq OWNED BY public.background_tracks.id;


--
-- Name: bible_texts; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.bible_texts (
    id bigint NOT NULL,
    book character varying NOT NULL,
    book_number integer NOT NULL,
    chapter integer NOT NULL,
    created_at timestamp(6) without time zone NOT NULL,
    text text NOT NULL,
    translation character varying DEFAULT 'nvi'::character varying,
    updated_at timestamp(6) without time zone NOT NULL,
    verse integer NOT NULL,
    CONSTRAINT bible_texts_scripture_coordinates_valid CHECK (((book_number > 0) AND (chapter > 0) AND (verse > 0)))
);


--
-- Name: bible_texts_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.bible_texts_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: bible_texts_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.bible_texts_id_seq OWNED BY public.bible_texts.id;


--
-- Name: bible_versions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.bible_versions (
    id bigint NOT NULL,
    code character varying NOT NULL,
    created_at timestamp(6) without time zone NOT NULL,
    description text,
    full_name character varying,
    is_active boolean DEFAULT true,
    is_recommended boolean DEFAULT false,
    language character varying DEFAULT 'pt-BR'::character varying,
    license character varying,
    name character varying NOT NULL,
    publisher character varying,
    updated_at timestamp(6) without time zone NOT NULL,
    versification_system character varying DEFAULT 'protestant'::character varying NOT NULL,
    year integer
);


--
-- Name: bible_versions_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.bible_versions_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: bible_versions_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.bible_versions_id_seq OWNED BY public.bible_versions.id;


--
-- Name: celebrations; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.celebrations (
    id bigint NOT NULL,
    calculation_rule character varying,
    can_be_transferred boolean DEFAULT true NOT NULL,
    celebration_type integer NOT NULL,
    created_at timestamp(6) without time zone NOT NULL,
    description text,
    description_year character varying,
    fixed_day integer,
    fixed_month integer,
    gender integer DEFAULT 0 NOT NULL,
    latin_name character varying,
    liturgical_color character varying,
    movable boolean DEFAULT false NOT NULL,
    name character varying NOT NULL,
    person_type integer DEFAULT 0 NOT NULL,
    post_slug character varying,
    prayer_book_id bigint NOT NULL,
    rank integer NOT NULL,
    transfer_rules jsonb DEFAULT '{}'::jsonb,
    updated_at timestamp(6) without time zone NOT NULL
);


--
-- Name: celebrations_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.celebrations_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: celebrations_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.celebrations_id_seq OWNED BY public.celebrations.id;


--
-- Name: collects; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.collects (
    id bigint NOT NULL,
    celebration_id bigint,
    created_at timestamp(6) without time zone NOT NULL,
    language_style character varying,
    prayer_book_id bigint NOT NULL,
    preface character varying,
    season_id bigint,
    sunday_reference character varying,
    text text,
    updated_at timestamp(6) without time zone NOT NULL
);


--
-- Name: collects_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.collects_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: collects_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.collects_id_seq OWNED BY public.collects.id;


--
-- Name: completions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.completions (
    id bigint NOT NULL,
    created_at timestamp(6) without time zone NOT NULL,
    date_reference date NOT NULL,
    duration_seconds integer,
    office_type character varying NOT NULL,
    prayer_book_id bigint,
    updated_at timestamp(6) without time zone NOT NULL,
    user_id bigint NOT NULL,
    CONSTRAINT completions_duration_non_negative CHECK (((duration_seconds IS NULL) OR (duration_seconds >= 0)))
);


--
-- Name: completions_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.completions_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: completions_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.completions_id_seq OWNED BY public.completions.id;


--
-- Name: custom_rosary_blocks; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.custom_rosary_blocks (
    id bigint NOT NULL,
    client_id character varying,
    created_at timestamp(6) without time zone NOT NULL,
    custom_rosary_prayer_id bigint NOT NULL,
    in_cycle boolean DEFAULT false NOT NULL,
    name character varying,
    "position" integer NOT NULL,
    repeat_count integer DEFAULT 1 NOT NULL,
    updated_at timestamp(6) without time zone NOT NULL
);


--
-- Name: custom_rosary_blocks_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.custom_rosary_blocks_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: custom_rosary_blocks_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.custom_rosary_blocks_id_seq OWNED BY public.custom_rosary_blocks.id;


--
-- Name: custom_rosary_prayers; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.custom_rosary_prayers (
    id bigint NOT NULL,
    client_id character varying NOT NULL,
    created_at timestamp(6) without time zone NOT NULL,
    cycle_repeat integer DEFAULT 1 NOT NULL,
    description text,
    is_public boolean DEFAULT false NOT NULL,
    last_moderation_reentry_at timestamp(6) without time zone,
    locale character varying DEFAULT 'pt-BR'::character varying NOT NULL,
    moderation_decision character varying,
    moderation_note text,
    moderation_reentry_count integer DEFAULT 0 NOT NULL,
    publication_attempts integer DEFAULT 0 NOT NULL,
    publication_category jsonb,
    publication_error text,
    publication_locale character varying,
    publication_retry_at timestamp(6) without time zone,
    publication_started_at timestamp(6) without time zone,
    publication_status character varying DEFAULT 'pending'::character varying NOT NULL,
    published_at timestamp(6) without time zone,
    reviewed_at timestamp(6) without time zone,
    share_status character varying DEFAULT 'private'::character varying NOT NULL,
    source_hash character varying,
    source_revision character varying,
    strapi_document_id character varying,
    strapi_slug character varying,
    title character varying NOT NULL,
    updated_at timestamp(6) without time zone NOT NULL,
    user_id bigint NOT NULL
);


--
-- Name: custom_rosary_prayers_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.custom_rosary_prayers_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: custom_rosary_prayers_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.custom_rosary_prayers_id_seq OWNED BY public.custom_rosary_prayers.id;


--
-- Name: custom_rosary_steps; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.custom_rosary_steps (
    id bigint NOT NULL,
    client_id character varying,
    created_at timestamp(6) without time zone NOT NULL,
    custom_rosary_block_id bigint NOT NULL,
    "position" integer NOT NULL,
    repeat_count integer DEFAULT 1 NOT NULL,
    step_type character varying NOT NULL,
    text text NOT NULL,
    title character varying NOT NULL,
    updated_at timestamp(6) without time zone NOT NULL
);


--
-- Name: custom_rosary_steps_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.custom_rosary_steps_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: custom_rosary_steps_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.custom_rosary_steps_id_seq OWNED BY public.custom_rosary_steps.id;


--
-- Name: developer_checkout_sessions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.developer_checkout_sessions (
    id bigint NOT NULL,
    created_at timestamp(6) without time zone NOT NULL,
    currency character varying NOT NULL,
    developer_id bigint NOT NULL,
    expires_at timestamp(6) without time zone NOT NULL,
    idempotency_key character varying NOT NULL,
    "interval" character varying NOT NULL,
    plan_code character varying NOT NULL,
    stripe_checkout_session_id character varying,
    trial_period_days integer DEFAULT 0 NOT NULL,
    updated_at timestamp(6) without time zone NOT NULL,
    url character varying
);


--
-- Name: developer_checkout_sessions_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.developer_checkout_sessions_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: developer_checkout_sessions_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.developer_checkout_sessions_id_seq OWNED BY public.developer_checkout_sessions.id;


--
-- Name: developer_subscriptions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.developer_subscriptions (
    id bigint NOT NULL,
    cancel_at_period_end boolean DEFAULT false NOT NULL,
    canceled_at timestamp(6) without time zone,
    created_at timestamp(6) without time zone NOT NULL,
    currency character varying DEFAULT 'brl'::character varying NOT NULL,
    current_period_end timestamp(6) without time zone,
    developer_id bigint NOT NULL,
    "interval" character varying DEFAULT 'month'::character varying NOT NULL,
    last_stripe_event_created_at timestamp(6) without time zone,
    plan_code character varying NOT NULL,
    status character varying NOT NULL,
    stripe_customer_id character varying NOT NULL,
    stripe_subscription_id character varying NOT NULL,
    trial_ends_at timestamp(6) without time zone,
    updated_at timestamp(6) without time zone NOT NULL
);


--
-- Name: developer_subscriptions_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.developer_subscriptions_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: developer_subscriptions_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.developer_subscriptions_id_seq OWNED BY public.developer_subscriptions.id;


--
-- Name: developers; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.developers (
    id bigint NOT NULL,
    approved boolean DEFAULT true NOT NULL,
    company_name character varying,
    created_at timestamp(6) without time zone NOT NULL,
    email character varying NOT NULL,
    legacy_free boolean DEFAULT false NOT NULL,
    max_keys integer DEFAULT 3 NOT NULL,
    name character varying,
    provider_uid character varying NOT NULL,
    stripe_customer_id character varying,
    updated_at timestamp(6) without time zone NOT NULL,
    use_case_description text,
    website character varying
);


--
-- Name: developers_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.developers_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: developers_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.developers_id_seq OWNED BY public.developers.id;


--
-- Name: fcm_tokens; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.fcm_tokens (
    id bigint NOT NULL,
    created_at timestamp(6) without time zone NOT NULL,
    platform character varying,
    token character varying NOT NULL,
    updated_at timestamp(6) without time zone NOT NULL,
    user_id bigint NOT NULL
);


--
-- Name: fcm_tokens_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.fcm_tokens_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: fcm_tokens_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.fcm_tokens_id_seq OWNED BY public.fcm_tokens.id;


--
-- Name: feature_flags; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.feature_flags (
    id bigint NOT NULL,
    created_at timestamp(6) without time zone NOT NULL,
    enabled boolean DEFAULT false NOT NULL,
    feature_key character varying NOT NULL,
    target_type character varying DEFAULT 'global'::character varying NOT NULL,
    updated_at timestamp(6) without time zone NOT NULL,
    user_id bigint,
    CONSTRAINT feature_flags_target_matches_user CHECK ((((target_type)::text = ANY (ARRAY[('global'::character varying)::text, ('user'::character varying)::text])) AND ((((target_type)::text = 'global'::text) AND (user_id IS NULL)) OR (((target_type)::text = 'user'::text) AND (user_id IS NOT NULL)))))
);


--
-- Name: feature_flags_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.feature_flags_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: feature_flags_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.feature_flags_id_seq OWNED BY public.feature_flags.id;


--
-- Name: journals; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.journals (
    id bigint NOT NULL,
    content text NOT NULL,
    created_at timestamp(6) without time zone NOT NULL,
    date_reference date NOT NULL,
    entry_type character varying NOT NULL,
    office_type character varying,
    updated_at timestamp(6) without time zone NOT NULL,
    user_id bigint NOT NULL
);


--
-- Name: journals_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.journals_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: journals_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.journals_id_seq OWNED BY public.journals.id;


--
-- Name: lectionary_readings; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.lectionary_readings (
    id bigint NOT NULL,
    celebration_id bigint,
    created_at timestamp(6) without time zone NOT NULL,
    cycle character varying,
    date_reference character varying,
    first_reading character varying,
    first_reading_abbreviated character varying,
    first_reading_alternative character varying,
    first_reading_title character varying,
    gospel character varying,
    gospel_abbreviated character varying,
    gospel_alternative character varying,
    gospel_title character varying,
    notes text,
    prayer_book_id bigint NOT NULL,
    psalm character varying,
    psalm_alternative character varying,
    psalm_title character varying,
    reading_type character varying,
    second_reading character varying,
    second_reading_abbreviated character varying,
    second_reading_alternative character varying,
    second_reading_title character varying,
    service_type character varying,
    service_variant character varying,
    updated_at timestamp(6) without time zone NOT NULL
);


--
-- Name: lectionary_readings_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.lectionary_readings_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: lectionary_readings_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.lectionary_readings_id_seq OWNED BY public.lectionary_readings.id;


--
-- Name: life_rule_exam_items; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.life_rule_exam_items (
    id bigint NOT NULL,
    client_id character varying NOT NULL,
    created_at timestamp(6) without time zone NOT NULL,
    life_rule_exam_id bigint NOT NULL,
    life_rule_step_id bigint,
    note text DEFAULT ''::text NOT NULL,
    rating character varying,
    step_description text DEFAULT ''::text NOT NULL,
    step_order integer DEFAULT 0 NOT NULL,
    step_title character varying NOT NULL,
    updated_at timestamp(6) without time zone NOT NULL
);


--
-- Name: life_rule_exam_items_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.life_rule_exam_items_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: life_rule_exam_items_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.life_rule_exam_items_id_seq OWNED BY public.life_rule_exam_items.id;


--
-- Name: life_rule_exams; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.life_rule_exams (
    id bigint NOT NULL,
    band character varying,
    client_id character varying NOT NULL,
    completed_at timestamp(6) without time zone,
    created_at timestamp(6) without time zone NOT NULL,
    focus_step_id bigint,
    intention text DEFAULT ''::text NOT NULL,
    life_rule_id bigint NOT NULL,
    life_rule_title character varying NOT NULL,
    period character varying DEFAULT 'monthly'::character varying NOT NULL,
    period_end date NOT NULL,
    period_start date NOT NULL,
    reflection text DEFAULT ''::text NOT NULL,
    score integer,
    season_name character varying,
    season_slug character varying,
    status character varying DEFAULT 'draft'::character varying NOT NULL,
    updated_at timestamp(6) without time zone NOT NULL,
    user_id bigint NOT NULL
);


--
-- Name: life_rule_exams_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.life_rule_exams_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: life_rule_exams_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.life_rule_exams_id_seq OWNED BY public.life_rule_exams.id;


--
-- Name: life_rule_steps; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.life_rule_steps (
    id bigint NOT NULL,
    created_at timestamp(6) without time zone NOT NULL,
    description text,
    life_rule_id bigint NOT NULL,
    "order" integer NOT NULL,
    title character varying NOT NULL,
    updated_at timestamp(6) without time zone NOT NULL,
    CONSTRAINT life_rule_steps_order_non_negative CHECK (("order" >= 0))
);


--
-- Name: life_rule_steps_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.life_rule_steps_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: life_rule_steps_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.life_rule_steps_id_seq OWNED BY public.life_rule_steps.id;


--
-- Name: life_rules; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.life_rules (
    id bigint NOT NULL,
    adoption_count integer DEFAULT 0 NOT NULL,
    approved boolean DEFAULT false NOT NULL,
    created_at timestamp(6) without time zone NOT NULL,
    description text,
    icon character varying NOT NULL,
    is_public boolean DEFAULT false NOT NULL,
    locale character varying(10) DEFAULT 'pt-BR'::character varying NOT NULL,
    original_life_rule_id bigint,
    title character varying NOT NULL,
    translation_key character varying NOT NULL,
    updated_at timestamp(6) without time zone NOT NULL,
    user_id bigint NOT NULL,
    CONSTRAINT life_rules_adoption_count_non_negative CHECK ((adoption_count >= 0))
);


--
-- Name: life_rules_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.life_rules_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: life_rules_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.life_rules_id_seq OWNED BY public.life_rules.id;


--
-- Name: liturgical_colors; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.liturgical_colors (
    id bigint NOT NULL,
    created_at timestamp(6) without time zone NOT NULL,
    hex_code character varying,
    name character varying,
    updated_at timestamp(6) without time zone NOT NULL,
    usage_description text
);


--
-- Name: liturgical_colors_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.liturgical_colors_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: liturgical_colors_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.liturgical_colors_id_seq OWNED BY public.liturgical_colors.id;


--
-- Name: liturgical_seasons; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.liturgical_seasons (
    id bigint NOT NULL,
    color character varying,
    created_at timestamp(6) without time zone NOT NULL,
    description text,
    name character varying,
    updated_at timestamp(6) without time zone NOT NULL
);


--
-- Name: liturgical_seasons_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.liturgical_seasons_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: liturgical_seasons_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.liturgical_seasons_id_seq OWNED BY public.liturgical_seasons.id;


--
-- Name: liturgical_texts; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.liturgical_texts (
    id bigint NOT NULL,
    audio_generation_status character varying DEFAULT 'pending'::character varying,
    audio_url character varying,
    audio_urls jsonb DEFAULT '{}'::jsonb NOT NULL,
    category character varying NOT NULL,
    content text NOT NULL,
    created_at timestamp(6) without time zone NOT NULL,
    prayer_book_id bigint NOT NULL,
    reference character varying,
    slug character varying NOT NULL,
    title character varying,
    updated_at timestamp(6) without time zone NOT NULL,
    CONSTRAINT liturgical_texts_audio_state_valid CHECK (((audio_generation_status IS NULL) OR ((audio_generation_status)::text = ANY (ARRAY[('pending'::character varying)::text, ('in_progress'::character varying)::text, ('processing'::character varying)::text, ('completed'::character varying)::text, ('partial'::character varying)::text, ('failed'::character varying)::text]))))
);


--
-- Name: liturgical_texts_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.liturgical_texts_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: liturgical_texts_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.liturgical_texts_id_seq OWNED BY public.liturgical_texts.id;


--
-- Name: notification_logs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.notification_logs (
    id bigint NOT NULL,
    body text,
    created_at timestamp(6) without time zone NOT NULL,
    data jsonb DEFAULT '{}'::jsonb,
    delivery_status jsonb DEFAULT '{}'::jsonb NOT NULL,
    error_message text,
    idempotency_key character varying,
    notification_type character varying,
    sent boolean DEFAULT false,
    title character varying,
    updated_at timestamp(6) without time zone NOT NULL,
    user_id bigint NOT NULL
);


--
-- Name: notification_logs_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.notification_logs_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: notification_logs_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.notification_logs_id_seq OWNED BY public.notification_logs.id;


--
-- Name: prayer_books; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.prayer_books (
    id bigint NOT NULL,
    code character varying NOT NULL,
    created_at timestamp(6) without time zone NOT NULL,
    description text,
    external_only boolean DEFAULT false NOT NULL,
    features jsonb DEFAULT '{}'::jsonb NOT NULL,
    image_url character varying,
    is_recommended boolean,
    jurisdiction character varying,
    language character varying DEFAULT 'pt-BR'::character varying NOT NULL,
    name character varying,
    "order" integer DEFAULT 0 NOT NULL,
    pdf_url character varying,
    premium_required boolean DEFAULT false NOT NULL,
    thumbnail_url character varying,
    updated_at timestamp(6) without time zone NOT NULL,
    year integer
);


--
-- Name: prayer_books_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.prayer_books_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: prayer_books_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.prayer_books_id_seq OWNED BY public.prayer_books.id;


--
-- Name: prayer_requests; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.prayer_requests (
    id bigint NOT NULL,
    content text,
    created_at timestamp(6) without time zone NOT NULL,
    "position" integer DEFAULT 0 NOT NULL,
    title character varying(255) NOT NULL,
    updated_at timestamp(6) without time zone NOT NULL,
    user_id bigint NOT NULL,
    week_start date NOT NULL,
    CONSTRAINT prayer_requests_position_non_negative CHECK (("position" >= 0))
);


--
-- Name: prayer_requests_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.prayer_requests_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: prayer_requests_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.prayer_requests_id_seq OWNED BY public.prayer_requests.id;


--
-- Name: preference_categories; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.preference_categories (
    id bigint NOT NULL,
    created_at timestamp(6) without time zone NOT NULL,
    description text,
    icon character varying,
    key character varying NOT NULL,
    name character varying NOT NULL,
    "position" integer DEFAULT 0 NOT NULL,
    prayer_book_id bigint NOT NULL,
    updated_at timestamp(6) without time zone NOT NULL
);


--
-- Name: preference_categories_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.preference_categories_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: preference_categories_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.preference_categories_id_seq OWNED BY public.preference_categories.id;


--
-- Name: preference_definitions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.preference_definitions (
    id bigint NOT NULL,
    created_at timestamp(6) without time zone NOT NULL,
    default_value character varying,
    depends_on jsonb,
    description text,
    key character varying NOT NULL,
    name character varying NOT NULL,
    options jsonb DEFAULT '[]'::jsonb,
    "position" integer DEFAULT 0 NOT NULL,
    pref_type character varying NOT NULL,
    preference_category_id bigint NOT NULL,
    required boolean DEFAULT false,
    simple_value character varying,
    updated_at timestamp(6) without time zone NOT NULL,
    validation_rules jsonb DEFAULT '{}'::jsonb
);


--
-- Name: preference_definitions_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.preference_definitions_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: preference_definitions_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.preference_definitions_id_seq OWNED BY public.preference_definitions.id;


--
-- Name: premium_subscription_events; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.premium_subscription_events (
    id bigint NOT NULL,
    created_at timestamp(6) without time zone NOT NULL,
    event_key character varying NOT NULL,
    event_type character varying NOT NULL,
    expires_at timestamp(6) without time zone,
    occurred_at timestamp(6) without time zone NOT NULL,
    product_identifier character varying,
    updated_at timestamp(6) without time zone NOT NULL,
    user_id bigint NOT NULL,
    will_renew boolean
);


--
-- Name: premium_subscription_events_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.premium_subscription_events_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: premium_subscription_events_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.premium_subscription_events_id_seq OWNED BY public.premium_subscription_events.id;


--
-- Name: psalm_cycles; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.psalm_cycles (
    id bigint NOT NULL,
    created_at timestamp(6) without time zone NOT NULL,
    cycle_type character varying NOT NULL,
    day_of_week integer NOT NULL,
    notes text,
    office_type character varying NOT NULL,
    prayer_book_id bigint NOT NULL,
    psalm_numbers jsonb DEFAULT '[]'::jsonb NOT NULL,
    updated_at timestamp(6) without time zone NOT NULL,
    week_number integer,
    CONSTRAINT psalm_cycles_calendar_values_valid CHECK ((((((cycle_type)::text = 'weekly'::text) AND (day_of_week >= 0) AND (day_of_week <= 6)) OR (((cycle_type)::text = 'monthly'::text) AND (day_of_week >= 1) AND (day_of_week <= 31))) AND ((week_number IS NULL) OR (week_number >= 1))))
);


--
-- Name: psalm_cycles_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.psalm_cycles_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: psalm_cycles_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.psalm_cycles_id_seq OWNED BY public.psalm_cycles.id;


--
-- Name: psalms; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.psalms (
    id bigint NOT NULL,
    antiphon text,
    created_at timestamp(6) without time zone NOT NULL,
    number integer NOT NULL,
    prayer_book_id bigint NOT NULL,
    title character varying,
    updated_at timestamp(6) without time zone NOT NULL,
    verses jsonb DEFAULT '{}'::jsonb NOT NULL,
    CONSTRAINT psalms_number_positive CHECK ((number > 0))
);


--
-- Name: psalms_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.psalms_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: psalms_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.psalms_id_seq OWNED BY public.psalms.id;


--
-- Name: schema_migrations; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.schema_migrations (
    version character varying NOT NULL
);


--
-- Name: shared_offices; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.shared_offices (
    id bigint NOT NULL,
    created_at timestamp(6) without time zone NOT NULL,
    date date NOT NULL,
    expires_at timestamp(6) without time zone NOT NULL,
    office_type character varying NOT NULL,
    prayer_book_code character varying NOT NULL,
    preferences jsonb DEFAULT '{}'::jsonb,
    seed bigint NOT NULL,
    short_code character varying NOT NULL,
    updated_at timestamp(6) without time zone NOT NULL,
    user_id bigint
);


--
-- Name: shared_offices_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.shared_offices_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: shared_offices_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.shared_offices_id_seq OWNED BY public.shared_offices.id;


--
-- Name: solid_cache_entries; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.solid_cache_entries (
    id bigint NOT NULL,
    byte_size integer NOT NULL,
    created_at timestamp(6) without time zone NOT NULL,
    key bytea NOT NULL,
    key_hash bigint NOT NULL,
    value bytea NOT NULL
);


--
-- Name: solid_cache_entries_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.solid_cache_entries_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: solid_cache_entries_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.solid_cache_entries_id_seq OWNED BY public.solid_cache_entries.id;


--
-- Name: solid_queue_blocked_executions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.solid_queue_blocked_executions (
    id bigint NOT NULL,
    concurrency_key character varying NOT NULL,
    created_at timestamp(6) without time zone NOT NULL,
    expires_at timestamp(6) without time zone NOT NULL,
    job_id bigint NOT NULL,
    priority integer DEFAULT 0 NOT NULL,
    queue_name character varying NOT NULL
);


--
-- Name: solid_queue_blocked_executions_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.solid_queue_blocked_executions_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: solid_queue_blocked_executions_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.solid_queue_blocked_executions_id_seq OWNED BY public.solid_queue_blocked_executions.id;


--
-- Name: solid_queue_claimed_executions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.solid_queue_claimed_executions (
    id bigint NOT NULL,
    created_at timestamp(6) without time zone NOT NULL,
    job_id bigint NOT NULL,
    process_id bigint
);


--
-- Name: solid_queue_claimed_executions_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.solid_queue_claimed_executions_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: solid_queue_claimed_executions_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.solid_queue_claimed_executions_id_seq OWNED BY public.solid_queue_claimed_executions.id;


--
-- Name: solid_queue_failed_executions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.solid_queue_failed_executions (
    id bigint NOT NULL,
    created_at timestamp(6) without time zone NOT NULL,
    error text,
    job_id bigint NOT NULL
);


--
-- Name: solid_queue_failed_executions_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.solid_queue_failed_executions_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: solid_queue_failed_executions_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.solid_queue_failed_executions_id_seq OWNED BY public.solid_queue_failed_executions.id;


--
-- Name: solid_queue_jobs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.solid_queue_jobs (
    id bigint NOT NULL,
    active_job_id character varying,
    arguments text,
    class_name character varying NOT NULL,
    concurrency_key character varying,
    created_at timestamp(6) without time zone NOT NULL,
    finished_at timestamp(6) without time zone,
    priority integer DEFAULT 0 NOT NULL,
    queue_name character varying NOT NULL,
    scheduled_at timestamp(6) without time zone,
    updated_at timestamp(6) without time zone NOT NULL
);


--
-- Name: solid_queue_jobs_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.solid_queue_jobs_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: solid_queue_jobs_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.solid_queue_jobs_id_seq OWNED BY public.solid_queue_jobs.id;


--
-- Name: solid_queue_pauses; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.solid_queue_pauses (
    id bigint NOT NULL,
    created_at timestamp(6) without time zone NOT NULL,
    queue_name character varying NOT NULL
);


--
-- Name: solid_queue_pauses_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.solid_queue_pauses_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: solid_queue_pauses_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.solid_queue_pauses_id_seq OWNED BY public.solid_queue_pauses.id;


--
-- Name: solid_queue_processes; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.solid_queue_processes (
    id bigint NOT NULL,
    created_at timestamp(6) without time zone NOT NULL,
    hostname character varying,
    kind character varying NOT NULL,
    last_heartbeat_at timestamp(6) without time zone NOT NULL,
    metadata text,
    name character varying NOT NULL,
    pid integer NOT NULL,
    supervisor_id bigint
);


--
-- Name: solid_queue_processes_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.solid_queue_processes_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: solid_queue_processes_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.solid_queue_processes_id_seq OWNED BY public.solid_queue_processes.id;


--
-- Name: solid_queue_ready_executions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.solid_queue_ready_executions (
    id bigint NOT NULL,
    created_at timestamp(6) without time zone NOT NULL,
    job_id bigint NOT NULL,
    priority integer DEFAULT 0 NOT NULL,
    queue_name character varying NOT NULL
);


--
-- Name: solid_queue_ready_executions_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.solid_queue_ready_executions_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: solid_queue_ready_executions_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.solid_queue_ready_executions_id_seq OWNED BY public.solid_queue_ready_executions.id;


--
-- Name: solid_queue_recurring_executions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.solid_queue_recurring_executions (
    id bigint NOT NULL,
    created_at timestamp(6) without time zone NOT NULL,
    job_id bigint NOT NULL,
    run_at timestamp(6) without time zone NOT NULL,
    task_key character varying NOT NULL
);


--
-- Name: solid_queue_recurring_executions_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.solid_queue_recurring_executions_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: solid_queue_recurring_executions_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.solid_queue_recurring_executions_id_seq OWNED BY public.solid_queue_recurring_executions.id;


--
-- Name: solid_queue_recurring_tasks; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.solid_queue_recurring_tasks (
    id bigint NOT NULL,
    arguments text,
    class_name character varying,
    command character varying(2048),
    created_at timestamp(6) without time zone NOT NULL,
    description text,
    key character varying NOT NULL,
    priority integer DEFAULT 0,
    queue_name character varying,
    schedule character varying NOT NULL,
    static boolean DEFAULT true NOT NULL,
    updated_at timestamp(6) without time zone NOT NULL
);


--
-- Name: solid_queue_recurring_tasks_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.solid_queue_recurring_tasks_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: solid_queue_recurring_tasks_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.solid_queue_recurring_tasks_id_seq OWNED BY public.solid_queue_recurring_tasks.id;


--
-- Name: solid_queue_scheduled_executions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.solid_queue_scheduled_executions (
    id bigint NOT NULL,
    created_at timestamp(6) without time zone NOT NULL,
    job_id bigint NOT NULL,
    priority integer DEFAULT 0 NOT NULL,
    queue_name character varying NOT NULL,
    scheduled_at timestamp(6) without time zone NOT NULL
);


--
-- Name: solid_queue_scheduled_executions_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.solid_queue_scheduled_executions_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: solid_queue_scheduled_executions_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.solid_queue_scheduled_executions_id_seq OWNED BY public.solid_queue_scheduled_executions.id;


--
-- Name: solid_queue_semaphores; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.solid_queue_semaphores (
    id bigint NOT NULL,
    created_at timestamp(6) without time zone NOT NULL,
    expires_at timestamp(6) without time zone NOT NULL,
    key character varying NOT NULL,
    updated_at timestamp(6) without time zone NOT NULL,
    value integer DEFAULT 1 NOT NULL
);


--
-- Name: solid_queue_semaphores_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.solid_queue_semaphores_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: solid_queue_semaphores_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.solid_queue_semaphores_id_seq OWNED BY public.solid_queue_semaphores.id;


--
-- Name: stripe_webhook_events; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.stripe_webhook_events (
    id bigint NOT NULL,
    created_at timestamp(6) without time zone NOT NULL,
    event_id character varying NOT NULL,
    event_type character varying NOT NULL,
    processed_at timestamp(6) without time zone,
    updated_at timestamp(6) without time zone NOT NULL
);


--
-- Name: stripe_webhook_events_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.stripe_webhook_events_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: stripe_webhook_events_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.stripe_webhook_events_id_seq OWNED BY public.stripe_webhook_events.id;


--
-- Name: user_audio_usages; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.user_audio_usages (
    id bigint NOT NULL,
    access_count integer DEFAULT 0 NOT NULL,
    asset_key character varying NOT NULL,
    audio_clip_id bigint,
    audio_type character varying NOT NULL,
    background_track_id bigint,
    created_at timestamp(6) without time zone NOT NULL,
    first_used_at timestamp(6) without time zone NOT NULL,
    last_used_at timestamp(6) without time zone NOT NULL,
    liturgical_text_id bigint,
    office_type character varying,
    prayer_book_code character varying,
    updated_at timestamp(6) without time zone NOT NULL,
    user_id bigint NOT NULL,
    voice character varying,
    CONSTRAINT user_audio_usages_access_count_positive CHECK ((access_count > 0)),
    CONSTRAINT user_audio_usages_audio_type_valid CHECK (((audio_type)::text = ANY (ARRAY[('background_track'::character varying)::text, ('audio_clip'::character varying)::text, ('liturgical_text'::character varying)::text])))
);


--
-- Name: user_audio_usages_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.user_audio_usages_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: user_audio_usages_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.user_audio_usages_id_seq OWNED BY public.user_audio_usages.id;


--
-- Name: user_background_track_favorites; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.user_background_track_favorites (
    id bigint NOT NULL,
    created_at timestamp(6) without time zone NOT NULL,
    track_id bigint NOT NULL,
    updated_at timestamp(6) without time zone NOT NULL,
    user_id bigint NOT NULL
);


--
-- Name: user_background_track_favorites_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.user_background_track_favorites_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: user_background_track_favorites_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.user_background_track_favorites_id_seq OWNED BY public.user_background_track_favorites.id;


--
-- Name: user_favorites; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.user_favorites (
    id bigint NOT NULL,
    created_at timestamp(6) without time zone NOT NULL,
    kind character varying NOT NULL,
    name character varying,
    post_slug character varying NOT NULL,
    updated_at timestamp(6) without time zone NOT NULL,
    user_id bigint NOT NULL
);


--
-- Name: user_favorites_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.user_favorites_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: user_favorites_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.user_favorites_id_seq OWNED BY public.user_favorites.id;


--
-- Name: user_onboardings; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.user_onboardings (
    id bigint NOT NULL,
    bible_version_id bigint NOT NULL,
    completed_at timestamp(6) without time zone,
    created_at timestamp(6) without time zone NOT NULL,
    mode character varying DEFAULT 'basic'::character varying NOT NULL,
    onboarding_completed boolean DEFAULT true,
    prayer_book_id bigint NOT NULL,
    preferences jsonb DEFAULT '{}'::jsonb NOT NULL,
    updated_at timestamp(6) without time zone NOT NULL,
    user_id bigint NOT NULL
);


--
-- Name: user_onboardings_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.user_onboardings_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: user_onboardings_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.user_onboardings_id_seq OWNED BY public.user_onboardings.id;


--
-- Name: users; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.users (
    id bigint NOT NULL,
    admin boolean DEFAULT false NOT NULL,
    country_code character varying(2),
    created_at timestamp(6) without time zone NOT NULL,
    current_streak integer DEFAULT 0,
    email character varying NOT NULL,
    last_completed_office_at timestamp(6) without time zone,
    longest_streak integer DEFAULT 0,
    name character varying,
    photo_url character varying,
    preferences jsonb DEFAULT '{}'::jsonb NOT NULL,
    premium_expires_at timestamp(6) without time zone,
    provider_uid character varying,
    revenue_cat_user_id character varying,
    timezone character varying DEFAULT 'America/Sao_Paulo'::character varying NOT NULL,
    updated_at timestamp(6) without time zone NOT NULL,
    CONSTRAINT users_streaks_non_negative CHECK (((current_streak >= 0) AND (longest_streak >= 0)))
);


--
-- Name: users_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.users_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: users_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.users_id_seq OWNED BY public.users.id;


--
-- Name: weekly_prayers; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.weekly_prayers (
    id bigint NOT NULL,
    created_at timestamp(6) without time zone NOT NULL,
    generated_at timestamp(6) without time zone NOT NULL,
    generated_prayer text NOT NULL,
    language character varying(10) DEFAULT 'pt-BR'::character varying NOT NULL,
    prayer_book_code character varying(50) NOT NULL,
    requests_digest character varying(32) NOT NULL,
    updated_at timestamp(6) without time zone NOT NULL,
    user_id bigint NOT NULL,
    week_start date NOT NULL
);


--
-- Name: weekly_prayers_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.weekly_prayers_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: weekly_prayers_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.weekly_prayers_id_seq OWNED BY public.weekly_prayers.id;


--
-- Name: active_storage_attachments id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.active_storage_attachments ALTER COLUMN id SET DEFAULT nextval('public.active_storage_attachments_id_seq'::regclass);


--
-- Name: active_storage_blobs id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.active_storage_blobs ALTER COLUMN id SET DEFAULT nextval('public.active_storage_blobs_id_seq'::regclass);


--
-- Name: active_storage_variant_records id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.active_storage_variant_records ALTER COLUMN id SET DEFAULT nextval('public.active_storage_variant_records_id_seq'::regclass);


--
-- Name: api_key_usage_logs id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.api_key_usage_logs ALTER COLUMN id SET DEFAULT nextval('public.api_key_usage_logs_id_seq'::regclass);


--
-- Name: api_keys id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.api_keys ALTER COLUMN id SET DEFAULT nextval('public.api_keys_id_seq'::regclass);


--
-- Name: audio_clip_candidates id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.audio_clip_candidates ALTER COLUMN id SET DEFAULT nextval('public.audio_clip_candidates_id_seq'::regclass);


--
-- Name: audio_clip_customizations id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.audio_clip_customizations ALTER COLUMN id SET DEFAULT nextval('public.audio_clip_customizations_id_seq'::regclass);


--
-- Name: audio_clip_usages id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.audio_clip_usages ALTER COLUMN id SET DEFAULT nextval('public.audio_clip_usages_id_seq'::regclass);


--
-- Name: audio_clips id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.audio_clips ALTER COLUMN id SET DEFAULT nextval('public.audio_clips_id_seq'::regclass);


--
-- Name: audio_generation_sessions id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.audio_generation_sessions ALTER COLUMN id SET DEFAULT nextval('public.audio_generation_sessions_id_seq'::regclass);


--
-- Name: audio_operations id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.audio_operations ALTER COLUMN id SET DEFAULT nextval('public.audio_operations_id_seq'::regclass);


--
-- Name: background_categories id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.background_categories ALTER COLUMN id SET DEFAULT nextval('public.background_categories_id_seq'::regclass);


--
-- Name: background_track_assets id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.background_track_assets ALTER COLUMN id SET DEFAULT nextval('public.background_track_assets_id_seq'::regclass);


--
-- Name: background_track_categories id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.background_track_categories ALTER COLUMN id SET DEFAULT nextval('public.background_track_categories_id_seq'::regclass);


--
-- Name: background_track_placements id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.background_track_placements ALTER COLUMN id SET DEFAULT nextval('public.background_track_placements_id_seq'::regclass);


--
-- Name: background_tracks id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.background_tracks ALTER COLUMN id SET DEFAULT nextval('public.background_tracks_id_seq'::regclass);


--
-- Name: bible_texts id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.bible_texts ALTER COLUMN id SET DEFAULT nextval('public.bible_texts_id_seq'::regclass);


--
-- Name: bible_versions id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.bible_versions ALTER COLUMN id SET DEFAULT nextval('public.bible_versions_id_seq'::regclass);


--
-- Name: celebrations id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.celebrations ALTER COLUMN id SET DEFAULT nextval('public.celebrations_id_seq'::regclass);


--
-- Name: collects id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.collects ALTER COLUMN id SET DEFAULT nextval('public.collects_id_seq'::regclass);


--
-- Name: completions id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.completions ALTER COLUMN id SET DEFAULT nextval('public.completions_id_seq'::regclass);


--
-- Name: custom_rosary_blocks id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.custom_rosary_blocks ALTER COLUMN id SET DEFAULT nextval('public.custom_rosary_blocks_id_seq'::regclass);


--
-- Name: custom_rosary_prayers id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.custom_rosary_prayers ALTER COLUMN id SET DEFAULT nextval('public.custom_rosary_prayers_id_seq'::regclass);


--
-- Name: custom_rosary_steps id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.custom_rosary_steps ALTER COLUMN id SET DEFAULT nextval('public.custom_rosary_steps_id_seq'::regclass);


--
-- Name: developer_checkout_sessions id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.developer_checkout_sessions ALTER COLUMN id SET DEFAULT nextval('public.developer_checkout_sessions_id_seq'::regclass);


--
-- Name: developer_subscriptions id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.developer_subscriptions ALTER COLUMN id SET DEFAULT nextval('public.developer_subscriptions_id_seq'::regclass);


--
-- Name: developers id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.developers ALTER COLUMN id SET DEFAULT nextval('public.developers_id_seq'::regclass);


--
-- Name: fcm_tokens id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.fcm_tokens ALTER COLUMN id SET DEFAULT nextval('public.fcm_tokens_id_seq'::regclass);


--
-- Name: feature_flags id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.feature_flags ALTER COLUMN id SET DEFAULT nextval('public.feature_flags_id_seq'::regclass);


--
-- Name: journals id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.journals ALTER COLUMN id SET DEFAULT nextval('public.journals_id_seq'::regclass);


--
-- Name: lectionary_readings id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.lectionary_readings ALTER COLUMN id SET DEFAULT nextval('public.lectionary_readings_id_seq'::regclass);


--
-- Name: life_rule_exam_items id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.life_rule_exam_items ALTER COLUMN id SET DEFAULT nextval('public.life_rule_exam_items_id_seq'::regclass);


--
-- Name: life_rule_exams id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.life_rule_exams ALTER COLUMN id SET DEFAULT nextval('public.life_rule_exams_id_seq'::regclass);


--
-- Name: life_rule_steps id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.life_rule_steps ALTER COLUMN id SET DEFAULT nextval('public.life_rule_steps_id_seq'::regclass);


--
-- Name: life_rules id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.life_rules ALTER COLUMN id SET DEFAULT nextval('public.life_rules_id_seq'::regclass);


--
-- Name: liturgical_colors id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.liturgical_colors ALTER COLUMN id SET DEFAULT nextval('public.liturgical_colors_id_seq'::regclass);


--
-- Name: liturgical_seasons id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.liturgical_seasons ALTER COLUMN id SET DEFAULT nextval('public.liturgical_seasons_id_seq'::regclass);


--
-- Name: liturgical_texts id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.liturgical_texts ALTER COLUMN id SET DEFAULT nextval('public.liturgical_texts_id_seq'::regclass);


--
-- Name: notification_logs id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.notification_logs ALTER COLUMN id SET DEFAULT nextval('public.notification_logs_id_seq'::regclass);


--
-- Name: prayer_books id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.prayer_books ALTER COLUMN id SET DEFAULT nextval('public.prayer_books_id_seq'::regclass);


--
-- Name: prayer_requests id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.prayer_requests ALTER COLUMN id SET DEFAULT nextval('public.prayer_requests_id_seq'::regclass);


--
-- Name: preference_categories id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.preference_categories ALTER COLUMN id SET DEFAULT nextval('public.preference_categories_id_seq'::regclass);


--
-- Name: preference_definitions id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.preference_definitions ALTER COLUMN id SET DEFAULT nextval('public.preference_definitions_id_seq'::regclass);


--
-- Name: premium_subscription_events id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.premium_subscription_events ALTER COLUMN id SET DEFAULT nextval('public.premium_subscription_events_id_seq'::regclass);


--
-- Name: psalm_cycles id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.psalm_cycles ALTER COLUMN id SET DEFAULT nextval('public.psalm_cycles_id_seq'::regclass);


--
-- Name: psalms id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.psalms ALTER COLUMN id SET DEFAULT nextval('public.psalms_id_seq'::regclass);


--
-- Name: shared_offices id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.shared_offices ALTER COLUMN id SET DEFAULT nextval('public.shared_offices_id_seq'::regclass);


--
-- Name: solid_cache_entries id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.solid_cache_entries ALTER COLUMN id SET DEFAULT nextval('public.solid_cache_entries_id_seq'::regclass);


--
-- Name: solid_queue_blocked_executions id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.solid_queue_blocked_executions ALTER COLUMN id SET DEFAULT nextval('public.solid_queue_blocked_executions_id_seq'::regclass);


--
-- Name: solid_queue_claimed_executions id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.solid_queue_claimed_executions ALTER COLUMN id SET DEFAULT nextval('public.solid_queue_claimed_executions_id_seq'::regclass);


--
-- Name: solid_queue_failed_executions id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.solid_queue_failed_executions ALTER COLUMN id SET DEFAULT nextval('public.solid_queue_failed_executions_id_seq'::regclass);


--
-- Name: solid_queue_jobs id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.solid_queue_jobs ALTER COLUMN id SET DEFAULT nextval('public.solid_queue_jobs_id_seq'::regclass);


--
-- Name: solid_queue_pauses id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.solid_queue_pauses ALTER COLUMN id SET DEFAULT nextval('public.solid_queue_pauses_id_seq'::regclass);


--
-- Name: solid_queue_processes id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.solid_queue_processes ALTER COLUMN id SET DEFAULT nextval('public.solid_queue_processes_id_seq'::regclass);


--
-- Name: solid_queue_ready_executions id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.solid_queue_ready_executions ALTER COLUMN id SET DEFAULT nextval('public.solid_queue_ready_executions_id_seq'::regclass);


--
-- Name: solid_queue_recurring_executions id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.solid_queue_recurring_executions ALTER COLUMN id SET DEFAULT nextval('public.solid_queue_recurring_executions_id_seq'::regclass);


--
-- Name: solid_queue_recurring_tasks id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.solid_queue_recurring_tasks ALTER COLUMN id SET DEFAULT nextval('public.solid_queue_recurring_tasks_id_seq'::regclass);


--
-- Name: solid_queue_scheduled_executions id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.solid_queue_scheduled_executions ALTER COLUMN id SET DEFAULT nextval('public.solid_queue_scheduled_executions_id_seq'::regclass);


--
-- Name: solid_queue_semaphores id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.solid_queue_semaphores ALTER COLUMN id SET DEFAULT nextval('public.solid_queue_semaphores_id_seq'::regclass);


--
-- Name: stripe_webhook_events id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.stripe_webhook_events ALTER COLUMN id SET DEFAULT nextval('public.stripe_webhook_events_id_seq'::regclass);


--
-- Name: user_audio_usages id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_audio_usages ALTER COLUMN id SET DEFAULT nextval('public.user_audio_usages_id_seq'::regclass);


--
-- Name: user_background_track_favorites id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_background_track_favorites ALTER COLUMN id SET DEFAULT nextval('public.user_background_track_favorites_id_seq'::regclass);


--
-- Name: user_favorites id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_favorites ALTER COLUMN id SET DEFAULT nextval('public.user_favorites_id_seq'::regclass);


--
-- Name: user_onboardings id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_onboardings ALTER COLUMN id SET DEFAULT nextval('public.user_onboardings_id_seq'::regclass);


--
-- Name: users id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.users ALTER COLUMN id SET DEFAULT nextval('public.users_id_seq'::regclass);


--
-- Name: weekly_prayers id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.weekly_prayers ALTER COLUMN id SET DEFAULT nextval('public.weekly_prayers_id_seq'::regclass);


--
-- Name: active_storage_attachments active_storage_attachments_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.active_storage_attachments
    ADD CONSTRAINT active_storage_attachments_pkey PRIMARY KEY (id);


--
-- Name: active_storage_blobs active_storage_blobs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.active_storage_blobs
    ADD CONSTRAINT active_storage_blobs_pkey PRIMARY KEY (id);


--
-- Name: active_storage_variant_records active_storage_variant_records_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.active_storage_variant_records
    ADD CONSTRAINT active_storage_variant_records_pkey PRIMARY KEY (id);


--
-- Name: api_key_usage_flushes api_key_usage_flushes_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.api_key_usage_flushes
    ADD CONSTRAINT api_key_usage_flushes_pkey PRIMARY KEY (id);


--
-- Name: api_key_usage_logs api_key_usage_logs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.api_key_usage_logs
    ADD CONSTRAINT api_key_usage_logs_pkey PRIMARY KEY (id);


--
-- Name: api_keys api_keys_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.api_keys
    ADD CONSTRAINT api_keys_pkey PRIMARY KEY (id);


--
-- Name: ar_internal_metadata ar_internal_metadata_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ar_internal_metadata
    ADD CONSTRAINT ar_internal_metadata_pkey PRIMARY KEY (key);


--
-- Name: audio_clip_candidates audio_clip_candidates_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.audio_clip_candidates
    ADD CONSTRAINT audio_clip_candidates_pkey PRIMARY KEY (id);


--
-- Name: audio_clip_customizations audio_clip_customizations_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.audio_clip_customizations
    ADD CONSTRAINT audio_clip_customizations_pkey PRIMARY KEY (id);


--
-- Name: audio_clip_usages audio_clip_usages_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.audio_clip_usages
    ADD CONSTRAINT audio_clip_usages_pkey PRIMARY KEY (id);


--
-- Name: audio_clips audio_clips_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.audio_clips
    ADD CONSTRAINT audio_clips_pkey PRIMARY KEY (id);


--
-- Name: audio_generation_sessions audio_generation_sessions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.audio_generation_sessions
    ADD CONSTRAINT audio_generation_sessions_pkey PRIMARY KEY (id);


--
-- Name: audio_operations audio_operations_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.audio_operations
    ADD CONSTRAINT audio_operations_pkey PRIMARY KEY (id);


--
-- Name: background_categories background_categories_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.background_categories
    ADD CONSTRAINT background_categories_pkey PRIMARY KEY (id);


--
-- Name: background_track_assets background_track_assets_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.background_track_assets
    ADD CONSTRAINT background_track_assets_pkey PRIMARY KEY (id);


--
-- Name: background_track_categories background_track_categories_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.background_track_categories
    ADD CONSTRAINT background_track_categories_pkey PRIMARY KEY (id);


--
-- Name: background_track_placements background_track_placements_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.background_track_placements
    ADD CONSTRAINT background_track_placements_pkey PRIMARY KEY (id);


--
-- Name: background_tracks background_tracks_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.background_tracks
    ADD CONSTRAINT background_tracks_pkey PRIMARY KEY (id);


--
-- Name: bible_texts bible_texts_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.bible_texts
    ADD CONSTRAINT bible_texts_pkey PRIMARY KEY (id);


--
-- Name: bible_versions bible_versions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.bible_versions
    ADD CONSTRAINT bible_versions_pkey PRIMARY KEY (id);


--
-- Name: celebrations celebrations_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.celebrations
    ADD CONSTRAINT celebrations_pkey PRIMARY KEY (id);


--
-- Name: collects collects_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.collects
    ADD CONSTRAINT collects_pkey PRIMARY KEY (id);


--
-- Name: completions completions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.completions
    ADD CONSTRAINT completions_pkey PRIMARY KEY (id);


--
-- Name: custom_rosary_blocks custom_rosary_blocks_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.custom_rosary_blocks
    ADD CONSTRAINT custom_rosary_blocks_pkey PRIMARY KEY (id);


--
-- Name: custom_rosary_prayers custom_rosary_prayers_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.custom_rosary_prayers
    ADD CONSTRAINT custom_rosary_prayers_pkey PRIMARY KEY (id);


--
-- Name: custom_rosary_steps custom_rosary_steps_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.custom_rosary_steps
    ADD CONSTRAINT custom_rosary_steps_pkey PRIMARY KEY (id);


--
-- Name: developer_checkout_sessions developer_checkout_sessions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.developer_checkout_sessions
    ADD CONSTRAINT developer_checkout_sessions_pkey PRIMARY KEY (id);


--
-- Name: developer_subscriptions developer_subscriptions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.developer_subscriptions
    ADD CONSTRAINT developer_subscriptions_pkey PRIMARY KEY (id);


--
-- Name: developers developers_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.developers
    ADD CONSTRAINT developers_pkey PRIMARY KEY (id);


--
-- Name: fcm_tokens fcm_tokens_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.fcm_tokens
    ADD CONSTRAINT fcm_tokens_pkey PRIMARY KEY (id);


--
-- Name: feature_flags feature_flags_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.feature_flags
    ADD CONSTRAINT feature_flags_pkey PRIMARY KEY (id);


--
-- Name: journals journals_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.journals
    ADD CONSTRAINT journals_pkey PRIMARY KEY (id);


--
-- Name: lectionary_readings lectionary_readings_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.lectionary_readings
    ADD CONSTRAINT lectionary_readings_pkey PRIMARY KEY (id);


--
-- Name: life_rule_exam_items life_rule_exam_items_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.life_rule_exam_items
    ADD CONSTRAINT life_rule_exam_items_pkey PRIMARY KEY (id);


--
-- Name: life_rule_exams life_rule_exams_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.life_rule_exams
    ADD CONSTRAINT life_rule_exams_pkey PRIMARY KEY (id);


--
-- Name: life_rule_steps life_rule_steps_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.life_rule_steps
    ADD CONSTRAINT life_rule_steps_pkey PRIMARY KEY (id);


--
-- Name: life_rules life_rules_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.life_rules
    ADD CONSTRAINT life_rules_pkey PRIMARY KEY (id);


--
-- Name: liturgical_colors liturgical_colors_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.liturgical_colors
    ADD CONSTRAINT liturgical_colors_pkey PRIMARY KEY (id);


--
-- Name: liturgical_seasons liturgical_seasons_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.liturgical_seasons
    ADD CONSTRAINT liturgical_seasons_pkey PRIMARY KEY (id);


--
-- Name: liturgical_texts liturgical_texts_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.liturgical_texts
    ADD CONSTRAINT liturgical_texts_pkey PRIMARY KEY (id);


--
-- Name: notification_logs notification_logs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.notification_logs
    ADD CONSTRAINT notification_logs_pkey PRIMARY KEY (id);


--
-- Name: prayer_books prayer_books_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.prayer_books
    ADD CONSTRAINT prayer_books_pkey PRIMARY KEY (id);


--
-- Name: prayer_requests prayer_requests_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.prayer_requests
    ADD CONSTRAINT prayer_requests_pkey PRIMARY KEY (id);


--
-- Name: preference_categories preference_categories_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.preference_categories
    ADD CONSTRAINT preference_categories_pkey PRIMARY KEY (id);


--
-- Name: preference_definitions preference_definitions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.preference_definitions
    ADD CONSTRAINT preference_definitions_pkey PRIMARY KEY (id);


--
-- Name: premium_subscription_events premium_subscription_events_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.premium_subscription_events
    ADD CONSTRAINT premium_subscription_events_pkey PRIMARY KEY (id);


--
-- Name: psalm_cycles psalm_cycles_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.psalm_cycles
    ADD CONSTRAINT psalm_cycles_pkey PRIMARY KEY (id);


--
-- Name: psalms psalms_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.psalms
    ADD CONSTRAINT psalms_pkey PRIMARY KEY (id);


--
-- Name: schema_migrations schema_migrations_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.schema_migrations
    ADD CONSTRAINT schema_migrations_pkey PRIMARY KEY (version);


--
-- Name: shared_offices shared_offices_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.shared_offices
    ADD CONSTRAINT shared_offices_pkey PRIMARY KEY (id);


--
-- Name: solid_cache_entries solid_cache_entries_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.solid_cache_entries
    ADD CONSTRAINT solid_cache_entries_pkey PRIMARY KEY (id);


--
-- Name: solid_queue_blocked_executions solid_queue_blocked_executions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.solid_queue_blocked_executions
    ADD CONSTRAINT solid_queue_blocked_executions_pkey PRIMARY KEY (id);


--
-- Name: solid_queue_claimed_executions solid_queue_claimed_executions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.solid_queue_claimed_executions
    ADD CONSTRAINT solid_queue_claimed_executions_pkey PRIMARY KEY (id);


--
-- Name: solid_queue_failed_executions solid_queue_failed_executions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.solid_queue_failed_executions
    ADD CONSTRAINT solid_queue_failed_executions_pkey PRIMARY KEY (id);


--
-- Name: solid_queue_jobs solid_queue_jobs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.solid_queue_jobs
    ADD CONSTRAINT solid_queue_jobs_pkey PRIMARY KEY (id);


--
-- Name: solid_queue_pauses solid_queue_pauses_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.solid_queue_pauses
    ADD CONSTRAINT solid_queue_pauses_pkey PRIMARY KEY (id);


--
-- Name: solid_queue_processes solid_queue_processes_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.solid_queue_processes
    ADD CONSTRAINT solid_queue_processes_pkey PRIMARY KEY (id);


--
-- Name: solid_queue_ready_executions solid_queue_ready_executions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.solid_queue_ready_executions
    ADD CONSTRAINT solid_queue_ready_executions_pkey PRIMARY KEY (id);


--
-- Name: solid_queue_recurring_executions solid_queue_recurring_executions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.solid_queue_recurring_executions
    ADD CONSTRAINT solid_queue_recurring_executions_pkey PRIMARY KEY (id);


--
-- Name: solid_queue_recurring_tasks solid_queue_recurring_tasks_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.solid_queue_recurring_tasks
    ADD CONSTRAINT solid_queue_recurring_tasks_pkey PRIMARY KEY (id);


--
-- Name: solid_queue_scheduled_executions solid_queue_scheduled_executions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.solid_queue_scheduled_executions
    ADD CONSTRAINT solid_queue_scheduled_executions_pkey PRIMARY KEY (id);


--
-- Name: solid_queue_semaphores solid_queue_semaphores_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.solid_queue_semaphores
    ADD CONSTRAINT solid_queue_semaphores_pkey PRIMARY KEY (id);


--
-- Name: stripe_webhook_events stripe_webhook_events_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.stripe_webhook_events
    ADD CONSTRAINT stripe_webhook_events_pkey PRIMARY KEY (id);


--
-- Name: user_audio_usages user_audio_usages_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_audio_usages
    ADD CONSTRAINT user_audio_usages_pkey PRIMARY KEY (id);


--
-- Name: user_background_track_favorites user_background_track_favorites_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_background_track_favorites
    ADD CONSTRAINT user_background_track_favorites_pkey PRIMARY KEY (id);


--
-- Name: user_favorites user_favorites_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_favorites
    ADD CONSTRAINT user_favorites_pkey PRIMARY KEY (id);


--
-- Name: user_onboardings user_onboardings_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_onboardings
    ADD CONSTRAINT user_onboardings_pkey PRIMARY KEY (id);


--
-- Name: users users_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_pkey PRIMARY KEY (id);


--
-- Name: weekly_prayers weekly_prayers_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.weekly_prayers
    ADD CONSTRAINT weekly_prayers_pkey PRIMARY KEY (id);


--
-- Name: idx_api_key_usage_by_date; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_api_key_usage_by_date ON public.api_key_usage_logs USING btree (api_key_id, date);


--
-- Name: idx_api_key_usage_unique; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_api_key_usage_unique ON public.api_key_usage_logs USING btree (api_key_id, endpoint, date);


--
-- Name: idx_lectionary_readings_pb_celebration; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_lectionary_readings_pb_celebration ON public.lectionary_readings USING btree (prayer_book_id, celebration_id);


--
-- Name: idx_liturgical_texts_pb_audio_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_liturgical_texts_pb_audio_status ON public.liturgical_texts USING btree (prayer_book_id, audio_generation_status);


--
-- Name: idx_on_date_office_type_prayer_book_code_ae0f27ea4d; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_on_date_office_type_prayer_book_code_ae0f27ea4d ON public.shared_offices USING btree (date, office_type, prayer_book_code);


--
-- Name: idx_on_stripe_checkout_session_id_aa63774c3a; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_on_stripe_checkout_session_id_aa63774c3a ON public.developer_checkout_sessions USING btree (stripe_checkout_session_id);


--
-- Name: idx_pref_definitions_on_category_and_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_pref_definitions_on_category_and_key ON public.preference_definitions USING btree (preference_category_id, key);


--
-- Name: idx_pref_definitions_on_category_and_position; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_pref_definitions_on_category_and_position ON public.preference_definitions USING btree (preference_category_id, "position");


--
-- Name: idx_weekly_prayers_user_week_pb; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_weekly_prayers_user_week_pb ON public.weekly_prayers USING btree (user_id, week_start, prayer_book_code);


--
-- Name: index_active_storage_attachments_on_blob_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_active_storage_attachments_on_blob_id ON public.active_storage_attachments USING btree (blob_id);


--
-- Name: index_active_storage_attachments_uniqueness; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX index_active_storage_attachments_uniqueness ON public.active_storage_attachments USING btree (record_type, record_id, name, blob_id);


--
-- Name: index_active_storage_blobs_on_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX index_active_storage_blobs_on_key ON public.active_storage_blobs USING btree (key);


--
-- Name: index_active_storage_variant_records_uniqueness; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX index_active_storage_variant_records_uniqueness ON public.active_storage_variant_records USING btree (blob_id, variation_digest);


--
-- Name: index_api_key_usage_flushes_on_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_api_key_usage_flushes_on_created_at ON public.api_key_usage_flushes USING btree (created_at);


--
-- Name: index_api_key_usage_logs_on_api_key_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_api_key_usage_logs_on_api_key_id ON public.api_key_usage_logs USING btree (api_key_id);


--
-- Name: index_api_keys_on_active; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_api_keys_on_active ON public.api_keys USING btree (active);


--
-- Name: index_api_keys_on_contact_email; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_api_keys_on_contact_email ON public.api_keys USING btree (contact_email);


--
-- Name: index_api_keys_on_developer_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_api_keys_on_developer_id ON public.api_keys USING btree (developer_id);


--
-- Name: index_api_keys_on_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX index_api_keys_on_key ON public.api_keys USING btree (key);


--
-- Name: index_audio_candidates_on_clip_and_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_audio_candidates_on_clip_and_status ON public.audio_clip_candidates USING btree (audio_clip_id, status);


--
-- Name: index_audio_clip_candidates_on_audio_clip_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_audio_clip_candidates_on_audio_clip_id ON public.audio_clip_candidates USING btree (audio_clip_id);


--
-- Name: index_audio_clip_candidates_on_audio_operation_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_audio_clip_candidates_on_audio_operation_id ON public.audio_clip_candidates USING btree (audio_operation_id);


--
-- Name: index_audio_clip_candidates_on_key; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_audio_clip_candidates_on_key ON public.audio_clip_candidates USING btree (key);


--
-- Name: index_audio_clip_candidates_on_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_audio_clip_candidates_on_status ON public.audio_clip_candidates USING btree (status);


--
-- Name: index_audio_clip_customizations_on_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_audio_clip_customizations_on_status ON public.audio_clip_customizations USING btree (status);


--
-- Name: index_audio_clip_usages_on_audio_clip_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_audio_clip_usages_on_audio_clip_id ON public.audio_clip_usages USING btree (audio_clip_id);


--
-- Name: index_audio_clip_usages_on_clip_book_source; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX index_audio_clip_usages_on_clip_book_source ON public.audio_clip_usages USING btree (audio_clip_id, prayer_book_code, source_name, source_key);


--
-- Name: index_audio_clip_usages_on_prayer_book_code; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_audio_clip_usages_on_prayer_book_code ON public.audio_clip_usages USING btree (prayer_book_code);


--
-- Name: index_audio_clip_usages_on_prayer_book_code_and_source_name; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_audio_clip_usages_on_prayer_book_code_and_source_name ON public.audio_clip_usages USING btree (prayer_book_code, source_name);


--
-- Name: index_audio_clips_on_configuration_fingerprint; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_audio_clips_on_configuration_fingerprint ON public.audio_clips USING btree (configuration_fingerprint);


--
-- Name: index_audio_clips_on_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_audio_clips_on_created_at ON public.audio_clips USING btree (created_at);


--
-- Name: index_audio_clips_on_custom_instructions_sha256; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_audio_clips_on_custom_instructions_sha256 ON public.audio_clips USING btree (custom_instructions_sha256);


--
-- Name: index_audio_clips_on_instructions_sha256; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_audio_clips_on_instructions_sha256 ON public.audio_clips USING btree (instructions_sha256);


--
-- Name: index_audio_clips_on_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX index_audio_clips_on_key ON public.audio_clips USING btree (key);


--
-- Name: index_audio_clips_on_kind; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_audio_clips_on_kind ON public.audio_clips USING btree (kind);


--
-- Name: index_audio_clips_on_language; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_audio_clips_on_language ON public.audio_clips USING btree (language);


--
-- Name: index_audio_clips_on_language_and_configuration_fingerprint; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_audio_clips_on_language_and_configuration_fingerprint ON public.audio_clips USING btree (language, configuration_fingerprint);


--
-- Name: index_audio_clips_on_provider_and_voice; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_audio_clips_on_provider_and_voice ON public.audio_clips USING btree (provider, voice);


--
-- Name: index_audio_customizations_on_text_and_profile; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX index_audio_customizations_on_text_and_profile ON public.audio_clip_customizations USING btree (text_digest, provider, model, voice, language);


--
-- Name: index_audio_generation_sessions_on_current_text_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_audio_generation_sessions_on_current_text_id ON public.audio_generation_sessions USING btree (current_text_id);


--
-- Name: index_audio_generation_sessions_on_prayer_book_code; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_audio_generation_sessions_on_prayer_book_code ON public.audio_generation_sessions USING btree (prayer_book_code);


--
-- Name: index_audio_generation_sessions_on_prayer_book_code_and_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_audio_generation_sessions_on_prayer_book_code_and_status ON public.audio_generation_sessions USING btree (prayer_book_code, status);


--
-- Name: index_audio_generation_sessions_on_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_audio_generation_sessions_on_status ON public.audio_generation_sessions USING btree (status);


--
-- Name: index_audio_operations_on_active_job_id; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX index_audio_operations_on_active_job_id ON public.audio_operations USING btree (active_job_id);


--
-- Name: index_audio_operations_on_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_audio_operations_on_created_at ON public.audio_operations USING btree (created_at);


--
-- Name: index_audio_operations_on_kind_and_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_audio_operations_on_kind_and_status ON public.audio_operations USING btree (kind, status);


--
-- Name: index_audio_operations_on_prayer_book_code; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_audio_operations_on_prayer_book_code ON public.audio_operations USING btree (prayer_book_code);


--
-- Name: index_audio_sessions_one_running_per_prayer_book; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX index_audio_sessions_one_running_per_prayer_book ON public.audio_generation_sessions USING btree (prayer_book_code) WHERE ((status)::text = 'running'::text);


--
-- Name: index_background_categories_on_slug; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX index_background_categories_on_slug ON public.background_categories USING btree (slug);


--
-- Name: index_background_track_assets_on_active_profile; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX index_background_track_assets_on_active_profile ON public.background_track_assets USING btree (track_id, profile) WHERE ((active = true) AND ((status)::text = 'ready'::text));


--
-- Name: index_background_track_assets_on_identity; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX index_background_track_assets_on_identity ON public.background_track_assets USING btree (track_id, profile, sha256);


--
-- Name: index_background_track_assets_on_object_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX index_background_track_assets_on_object_key ON public.background_track_assets USING btree (object_key);


--
-- Name: index_background_track_assets_on_track_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_background_track_assets_on_track_id ON public.background_track_assets USING btree (track_id);


--
-- Name: index_background_track_categories_on_category_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_background_track_categories_on_category_id ON public.background_track_categories USING btree (category_id);


--
-- Name: index_background_track_categories_on_track_and_category; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX index_background_track_categories_on_track_and_category ON public.background_track_categories USING btree (track_id, category_id);


--
-- Name: index_background_track_categories_on_track_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_background_track_categories_on_track_id ON public.background_track_categories USING btree (track_id);


--
-- Name: index_background_track_placements_on_identity; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX index_background_track_placements_on_identity ON public.background_track_placements USING btree (track_id, facet, key, prayer_book_code) NULLS NOT DISTINCT;


--
-- Name: index_background_track_placements_on_lookup; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_background_track_placements_on_lookup ON public.background_track_placements USING btree (facet, key, prayer_book_code, priority);


--
-- Name: index_background_track_placements_on_track_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_background_track_placements_on_track_id ON public.background_track_placements USING btree (track_id);


--
-- Name: index_background_tracks_on_catalogue_order; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_background_tracks_on_catalogue_order ON public.background_tracks USING btree (status, kind, sort_order);


--
-- Name: index_background_tracks_on_slug; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX index_background_tracks_on_slug ON public.background_tracks USING btree (slug);


--
-- Name: index_bible_texts_on_book_chapter_trans_verse_optimized; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_bible_texts_on_book_chapter_trans_verse_optimized ON public.bible_texts USING btree (book, chapter, translation, verse);


--
-- Name: index_bible_texts_on_book_chapter_translation; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_bible_texts_on_book_chapter_translation ON public.bible_texts USING btree (book, chapter, translation);


--
-- Name: index_bible_texts_on_book_number_lookup; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_bible_texts_on_book_number_lookup ON public.bible_texts USING btree (book_number, chapter, verse, translation);


--
-- Name: index_bible_texts_on_verse_lookup; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_bible_texts_on_verse_lookup ON public.bible_texts USING btree (book, chapter, verse, translation);


--
-- Name: index_bible_versions_on_code; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX index_bible_versions_on_code ON public.bible_versions USING btree (code);


--
-- Name: index_bible_versions_on_is_active_and_is_recommended; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_bible_versions_on_is_active_and_is_recommended ON public.bible_versions USING btree (is_active, is_recommended);


--
-- Name: index_bible_versions_on_language; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_bible_versions_on_language ON public.bible_versions USING btree (language);


--
-- Name: index_bible_versions_on_language_active_recommended_name; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_bible_versions_on_language_active_recommended_name ON public.bible_versions USING btree (language, is_active, is_recommended, name);


--
-- Name: index_bible_versions_on_versification_system; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_bible_versions_on_versification_system ON public.bible_versions USING btree (versification_system);


--
-- Name: index_celebrations_on_calculation_rule; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_celebrations_on_calculation_rule ON public.celebrations USING btree (calculation_rule);


--
-- Name: index_celebrations_on_celebration_type; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_celebrations_on_celebration_type ON public.celebrations USING btree (celebration_type);


--
-- Name: index_celebrations_on_fixed_month_and_fixed_day; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_celebrations_on_fixed_month_and_fixed_day ON public.celebrations USING btree (fixed_month, fixed_day);


--
-- Name: index_celebrations_on_gender; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_celebrations_on_gender ON public.celebrations USING btree (gender);


--
-- Name: index_celebrations_on_movable; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_celebrations_on_movable ON public.celebrations USING btree (movable);


--
-- Name: index_celebrations_on_name_and_prayer_book_id; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX index_celebrations_on_name_and_prayer_book_id ON public.celebrations USING btree (name, prayer_book_id);


--
-- Name: index_celebrations_on_person_type; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_celebrations_on_person_type ON public.celebrations USING btree (person_type);


--
-- Name: index_celebrations_on_post_slug; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_celebrations_on_post_slug ON public.celebrations USING btree (post_slug);


--
-- Name: index_celebrations_on_prayer_book_and_movable; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_celebrations_on_prayer_book_and_movable ON public.celebrations USING btree (prayer_book_id, movable);


--
-- Name: index_celebrations_on_prayer_book_and_rank; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_celebrations_on_prayer_book_and_rank ON public.celebrations USING btree (prayer_book_id, rank);


--
-- Name: index_celebrations_on_prayer_book_and_transferable; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_celebrations_on_prayer_book_and_transferable ON public.celebrations USING btree (prayer_book_id, can_be_transferred);


--
-- Name: index_celebrations_on_prayer_book_and_type; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_celebrations_on_prayer_book_and_type ON public.celebrations USING btree (prayer_book_id, celebration_type);


--
-- Name: index_celebrations_on_prayer_book_date; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_celebrations_on_prayer_book_date ON public.celebrations USING btree (prayer_book_id, fixed_month, fixed_day) WHERE (movable = false);


--
-- Name: index_celebrations_on_rank; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_celebrations_on_rank ON public.celebrations USING btree (rank);


--
-- Name: index_collects_on_book_sunday_language_style; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_collects_on_book_sunday_language_style ON public.collects USING btree (prayer_book_id, sunday_reference, language_style);


--
-- Name: index_collects_on_celebration_id_and_prayer_book_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_collects_on_celebration_id_and_prayer_book_id ON public.collects USING btree (celebration_id, prayer_book_id);


--
-- Name: index_collects_on_prayer_book_and_season; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_collects_on_prayer_book_and_season ON public.collects USING btree (prayer_book_id, season_id);


--
-- Name: index_collects_on_prayer_book_and_sunday_ref; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_collects_on_prayer_book_and_sunday_ref ON public.collects USING btree (prayer_book_id, sunday_reference);


--
-- Name: index_collects_on_season_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_collects_on_season_id ON public.collects USING btree (season_id);


--
-- Name: index_completions_on_date_reference; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_completions_on_date_reference ON public.completions USING btree (date_reference DESC);


--
-- Name: index_completions_on_prayer_book_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_completions_on_prayer_book_id ON public.completions USING btree (prayer_book_id);


--
-- Name: index_completions_on_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_completions_on_user_id ON public.completions USING btree (user_id);


--
-- Name: index_completions_unique; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX index_completions_unique ON public.completions USING btree (user_id, date_reference, office_type);


--
-- Name: index_custom_rosary_blocks_on_prayer; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_custom_rosary_blocks_on_prayer ON public.custom_rosary_blocks USING btree (custom_rosary_prayer_id);


--
-- Name: index_custom_rosary_blocks_on_prayer_and_position; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_custom_rosary_blocks_on_prayer_and_position ON public.custom_rosary_blocks USING btree (custom_rosary_prayer_id, "position");


--
-- Name: index_custom_rosary_prayers_on_public_share_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_custom_rosary_prayers_on_public_share_status ON public.custom_rosary_prayers USING btree (share_status) WHERE is_public;


--
-- Name: index_custom_rosary_prayers_on_publication_retry_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_custom_rosary_prayers_on_publication_retry_at ON public.custom_rosary_prayers USING btree (publication_retry_at);


--
-- Name: index_custom_rosary_prayers_on_publication_started_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_custom_rosary_prayers_on_publication_started_at ON public.custom_rosary_prayers USING btree (publication_started_at);


--
-- Name: index_custom_rosary_prayers_on_publication_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_custom_rosary_prayers_on_publication_status ON public.custom_rosary_prayers USING btree (publication_status);


--
-- Name: index_custom_rosary_prayers_on_share_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_custom_rosary_prayers_on_share_status ON public.custom_rosary_prayers USING btree (share_status);


--
-- Name: index_custom_rosary_prayers_on_source_hash_and_publication; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_custom_rosary_prayers_on_source_hash_and_publication ON public.custom_rosary_prayers USING btree (source_hash, publication_status);


--
-- Name: index_custom_rosary_prayers_on_strapi_document_id; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX index_custom_rosary_prayers_on_strapi_document_id ON public.custom_rosary_prayers USING btree (strapi_document_id);


--
-- Name: index_custom_rosary_prayers_on_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_custom_rosary_prayers_on_user_id ON public.custom_rosary_prayers USING btree (user_id);


--
-- Name: index_custom_rosary_prayers_on_user_id_and_client_id; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX index_custom_rosary_prayers_on_user_id_and_client_id ON public.custom_rosary_prayers USING btree (user_id, client_id);


--
-- Name: index_custom_rosary_steps_on_block; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_custom_rosary_steps_on_block ON public.custom_rosary_steps USING btree (custom_rosary_block_id);


--
-- Name: index_custom_rosary_steps_on_block_and_position; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_custom_rosary_steps_on_block_and_position ON public.custom_rosary_steps USING btree (custom_rosary_block_id, "position");


--
-- Name: index_developer_checkout_sessions_on_developer_id; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX index_developer_checkout_sessions_on_developer_id ON public.developer_checkout_sessions USING btree (developer_id);


--
-- Name: index_developer_checkout_sessions_on_idempotency_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX index_developer_checkout_sessions_on_idempotency_key ON public.developer_checkout_sessions USING btree (idempotency_key);


--
-- Name: index_developer_subscriptions_on_developer_id; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX index_developer_subscriptions_on_developer_id ON public.developer_subscriptions USING btree (developer_id);


--
-- Name: index_developer_subscriptions_on_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_developer_subscriptions_on_status ON public.developer_subscriptions USING btree (status);


--
-- Name: index_developer_subscriptions_on_stripe_subscription_id; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX index_developer_subscriptions_on_stripe_subscription_id ON public.developer_subscriptions USING btree (stripe_subscription_id);


--
-- Name: index_developers_on_email; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX index_developers_on_email ON public.developers USING btree (email);


--
-- Name: index_developers_on_provider_uid; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX index_developers_on_provider_uid ON public.developers USING btree (provider_uid);


--
-- Name: index_developers_on_stripe_customer_id; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX index_developers_on_stripe_customer_id ON public.developers USING btree (stripe_customer_id);


--
-- Name: index_fcm_tokens_on_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_fcm_tokens_on_user_id ON public.fcm_tokens USING btree (user_id);


--
-- Name: index_fcm_tokens_on_user_id_and_token; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX index_fcm_tokens_on_user_id_and_token ON public.fcm_tokens USING btree (user_id, token);


--
-- Name: index_feature_flags_on_key_and_global_target; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX index_feature_flags_on_key_and_global_target ON public.feature_flags USING btree (feature_key, target_type) WHERE ((target_type)::text = 'global'::text);


--
-- Name: index_feature_flags_on_key_and_user_target; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX index_feature_flags_on_key_and_user_target ON public.feature_flags USING btree (feature_key, target_type, user_id) WHERE ((target_type)::text = 'user'::text);


--
-- Name: index_feature_flags_on_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_feature_flags_on_user_id ON public.feature_flags USING btree (user_id);


--
-- Name: index_journals_on_date_reference; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_journals_on_date_reference ON public.journals USING btree (date_reference DESC);


--
-- Name: index_journals_on_user_date_type_office; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_journals_on_user_date_type_office ON public.journals USING btree (user_id, date_reference, entry_type, office_type);


--
-- Name: index_journals_on_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_journals_on_user_id ON public.journals USING btree (user_id);


--
-- Name: index_journals_on_user_id_and_date_reference; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_journals_on_user_id_and_date_reference ON public.journals USING btree (user_id, date_reference);


--
-- Name: index_lectionary_on_pb_ref_cycle_service; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_lectionary_on_pb_ref_cycle_service ON public.lectionary_readings USING btree (prayer_book_id, date_reference, cycle, service_type);


--
-- Name: index_lectionary_on_pb_ref_cycle_service_variant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_lectionary_on_pb_ref_cycle_service_variant ON public.lectionary_readings USING btree (prayer_book_id, date_reference, cycle, service_type, service_variant);


--
-- Name: index_lectionary_readings_on_celebration_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_lectionary_readings_on_celebration_id ON public.lectionary_readings USING btree (celebration_id);


--
-- Name: index_lectionary_readings_on_common_lookup; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_lectionary_readings_on_common_lookup ON public.lectionary_readings USING btree (cycle, service_type, prayer_book_id, date_reference);


--
-- Name: index_lectionary_readings_on_date_service_prayer_book; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_lectionary_readings_on_date_service_prayer_book ON public.lectionary_readings USING btree (date_reference, service_type, prayer_book_id);


--
-- Name: index_lectionary_readings_on_reading_type; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_lectionary_readings_on_reading_type ON public.lectionary_readings USING btree (reading_type);


--
-- Name: index_life_rule_exam_items_on_life_rule_exam_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_life_rule_exam_items_on_life_rule_exam_id ON public.life_rule_exam_items USING btree (life_rule_exam_id);


--
-- Name: index_life_rule_exam_items_on_life_rule_exam_id_and_client_id; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX index_life_rule_exam_items_on_life_rule_exam_id_and_client_id ON public.life_rule_exam_items USING btree (life_rule_exam_id, client_id);


--
-- Name: index_life_rule_exam_items_on_life_rule_exam_id_and_step_order; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_life_rule_exam_items_on_life_rule_exam_id_and_step_order ON public.life_rule_exam_items USING btree (life_rule_exam_id, step_order);


--
-- Name: index_life_rule_exam_items_on_life_rule_step_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_life_rule_exam_items_on_life_rule_step_id ON public.life_rule_exam_items USING btree (life_rule_step_id);


--
-- Name: index_life_rule_exams_on_life_rule_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_life_rule_exams_on_life_rule_id ON public.life_rule_exams USING btree (life_rule_id);


--
-- Name: index_life_rule_exams_on_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_life_rule_exams_on_user_id ON public.life_rule_exams USING btree (user_id);


--
-- Name: index_life_rule_exams_on_user_id_and_client_id; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX index_life_rule_exams_on_user_id_and_client_id ON public.life_rule_exams USING btree (user_id, client_id);


--
-- Name: index_life_rule_exams_on_user_id_and_completed_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_life_rule_exams_on_user_id_and_completed_at ON public.life_rule_exams USING btree (user_id, completed_at);


--
-- Name: index_life_rule_exams_on_user_id_and_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_life_rule_exams_on_user_id_and_status ON public.life_rule_exams USING btree (user_id, status);


--
-- Name: index_life_rule_steps_on_life_rule_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_life_rule_steps_on_life_rule_id ON public.life_rule_steps USING btree (life_rule_id);


--
-- Name: index_life_rule_steps_on_life_rule_id_and_order; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_life_rule_steps_on_life_rule_id_and_order ON public.life_rule_steps USING btree (life_rule_id, "order");


--
-- Name: index_life_rules_on_adoption_count; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_life_rules_on_adoption_count ON public.life_rules USING btree (adoption_count);


--
-- Name: index_life_rules_on_approved; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_life_rules_on_approved ON public.life_rules USING btree (approved);


--
-- Name: index_life_rules_on_is_public; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_life_rules_on_is_public ON public.life_rules USING btree (is_public);


--
-- Name: index_life_rules_on_locale; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_life_rules_on_locale ON public.life_rules USING btree (locale);


--
-- Name: index_life_rules_on_lower_title; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_life_rules_on_lower_title ON public.life_rules USING btree (lower((title)::text));


--
-- Name: index_life_rules_on_original_life_rule_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_life_rules_on_original_life_rule_id ON public.life_rules USING btree (original_life_rule_id);


--
-- Name: index_life_rules_on_public_approved; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_life_rules_on_public_approved ON public.life_rules USING btree (is_public, approved) WHERE ((is_public = true) AND (approved = true));


--
-- Name: index_life_rules_on_translation_key_and_locale; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX index_life_rules_on_translation_key_and_locale ON public.life_rules USING btree (translation_key, locale);


--
-- Name: index_life_rules_on_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX index_life_rules_on_user_id ON public.life_rules USING btree (user_id);


--
-- Name: index_liturgical_texts_on_audio_generation_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_liturgical_texts_on_audio_generation_status ON public.liturgical_texts USING btree (audio_generation_status);


--
-- Name: index_liturgical_texts_on_audio_urls; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_liturgical_texts_on_audio_urls ON public.liturgical_texts USING gin (audio_urls);


--
-- Name: index_liturgical_texts_on_category; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_liturgical_texts_on_category ON public.liturgical_texts USING btree (category);


--
-- Name: index_liturgical_texts_on_prayer_book_and_category; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_liturgical_texts_on_prayer_book_and_category ON public.liturgical_texts USING btree (prayer_book_id, category);


--
-- Name: index_liturgical_texts_on_slug_and_prayer_book_id; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX index_liturgical_texts_on_slug_and_prayer_book_id ON public.liturgical_texts USING btree (slug, prayer_book_id);


--
-- Name: index_notification_logs_on_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_notification_logs_on_created_at ON public.notification_logs USING btree (created_at);


--
-- Name: index_notification_logs_on_idempotency_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX index_notification_logs_on_idempotency_key ON public.notification_logs USING btree (idempotency_key) WHERE (idempotency_key IS NOT NULL);


--
-- Name: index_notification_logs_on_notification_type; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_notification_logs_on_notification_type ON public.notification_logs USING btree (notification_type);


--
-- Name: index_notification_logs_on_sent; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_notification_logs_on_sent ON public.notification_logs USING btree (sent);


--
-- Name: index_notification_logs_on_type_and_created; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_notification_logs_on_type_and_created ON public.notification_logs USING btree (notification_type, created_at DESC);


--
-- Name: index_notification_logs_on_user_and_sent; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_notification_logs_on_user_and_sent ON public.notification_logs USING btree (user_id, sent);


--
-- Name: index_notification_logs_on_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_notification_logs_on_user_id ON public.notification_logs USING btree (user_id);


--
-- Name: index_prayer_books_on_code; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX index_prayer_books_on_code ON public.prayer_books USING btree (code);


--
-- Name: index_prayer_books_on_external_only; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_prayer_books_on_external_only ON public.prayer_books USING btree (external_only);


--
-- Name: index_prayer_books_on_features; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_prayer_books_on_features ON public.prayer_books USING gin (features);


--
-- Name: index_prayer_books_on_is_recommended; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_prayer_books_on_is_recommended ON public.prayer_books USING btree (is_recommended);


--
-- Name: index_prayer_books_on_language; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_prayer_books_on_language ON public.prayer_books USING btree (language);


--
-- Name: index_prayer_books_on_language_and_code; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_prayer_books_on_language_and_code ON public.prayer_books USING btree (language, code);


--
-- Name: index_prayer_books_on_premium_required; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_prayer_books_on_premium_required ON public.prayer_books USING btree (premium_required);


--
-- Name: index_prayer_books_on_premium_required_and_is_recommended; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_prayer_books_on_premium_required_and_is_recommended ON public.prayer_books USING btree (premium_required, is_recommended);


--
-- Name: index_prayer_requests_on_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_prayer_requests_on_user_id ON public.prayer_requests USING btree (user_id);


--
-- Name: index_prayer_requests_on_user_id_and_week_start; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_prayer_requests_on_user_id_and_week_start ON public.prayer_requests USING btree (user_id, week_start);


--
-- Name: index_prayer_requests_on_user_week_position; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_prayer_requests_on_user_week_position ON public.prayer_requests USING btree (user_id, week_start, "position");


--
-- Name: index_preference_categories_on_prayer_book_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_preference_categories_on_prayer_book_id ON public.preference_categories USING btree (prayer_book_id);


--
-- Name: index_preference_categories_on_prayer_book_id_and_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX index_preference_categories_on_prayer_book_id_and_key ON public.preference_categories USING btree (prayer_book_id, key);


--
-- Name: index_preference_categories_on_prayer_book_id_and_position; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_preference_categories_on_prayer_book_id_and_position ON public.preference_categories USING btree (prayer_book_id, "position");


--
-- Name: index_preference_definitions_on_pref_type; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_preference_definitions_on_pref_type ON public.preference_definitions USING btree (pref_type);


--
-- Name: index_preference_definitions_on_preference_category_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_preference_definitions_on_preference_category_id ON public.preference_definitions USING btree (preference_category_id);


--
-- Name: index_premium_events_on_type_and_occurred_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_premium_events_on_type_and_occurred_at ON public.premium_subscription_events USING btree (event_type, occurred_at);


--
-- Name: index_premium_events_on_user_and_occurred_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_premium_events_on_user_and_occurred_at ON public.premium_subscription_events USING btree (user_id, occurred_at);


--
-- Name: index_premium_subscription_events_on_event_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX index_premium_subscription_events_on_event_key ON public.premium_subscription_events USING btree (event_key);


--
-- Name: index_premium_subscription_events_on_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_premium_subscription_events_on_user_id ON public.premium_subscription_events USING btree (user_id);


--
-- Name: index_psalm_cycles_on_cycle_lookup_with_prayer_book; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX index_psalm_cycles_on_cycle_lookup_with_prayer_book ON public.psalm_cycles USING btree (cycle_type, week_number, day_of_week, office_type, prayer_book_id);


--
-- Name: index_psalm_cycles_on_prayer_book_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_psalm_cycles_on_prayer_book_id ON public.psalm_cycles USING btree (prayer_book_id);


--
-- Name: index_psalms_on_number_and_prayer_book_id; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX index_psalms_on_number_and_prayer_book_id ON public.psalms USING btree (number, prayer_book_id);


--
-- Name: index_psalms_on_prayer_book_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_psalms_on_prayer_book_id ON public.psalms USING btree (prayer_book_id);


--
-- Name: index_shared_offices_on_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_shared_offices_on_created_at ON public.shared_offices USING btree (created_at);


--
-- Name: index_shared_offices_on_expires_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_shared_offices_on_expires_at ON public.shared_offices USING btree (expires_at);


--
-- Name: index_shared_offices_on_short_code; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX index_shared_offices_on_short_code ON public.shared_offices USING btree (short_code);


--
-- Name: index_shared_offices_on_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_shared_offices_on_user_id ON public.shared_offices USING btree (user_id);


--
-- Name: index_solid_cache_entries_on_byte_size; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_solid_cache_entries_on_byte_size ON public.solid_cache_entries USING btree (byte_size);


--
-- Name: index_solid_cache_entries_on_key_hash; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX index_solid_cache_entries_on_key_hash ON public.solid_cache_entries USING btree (key_hash);


--
-- Name: index_solid_cache_entries_on_key_hash_and_byte_size; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_solid_cache_entries_on_key_hash_and_byte_size ON public.solid_cache_entries USING btree (key_hash, byte_size);


--
-- Name: index_solid_queue_blocked_executions_for_maintenance; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_solid_queue_blocked_executions_for_maintenance ON public.solid_queue_blocked_executions USING btree (expires_at, concurrency_key);


--
-- Name: index_solid_queue_blocked_executions_for_release; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_solid_queue_blocked_executions_for_release ON public.solid_queue_blocked_executions USING btree (concurrency_key, priority, job_id);


--
-- Name: index_solid_queue_blocked_executions_on_job_id; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX index_solid_queue_blocked_executions_on_job_id ON public.solid_queue_blocked_executions USING btree (job_id);


--
-- Name: index_solid_queue_claimed_executions_on_job_id; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX index_solid_queue_claimed_executions_on_job_id ON public.solid_queue_claimed_executions USING btree (job_id);


--
-- Name: index_solid_queue_claimed_executions_on_process_id_and_job_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_solid_queue_claimed_executions_on_process_id_and_job_id ON public.solid_queue_claimed_executions USING btree (process_id, job_id);


--
-- Name: index_solid_queue_dispatch_all; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_solid_queue_dispatch_all ON public.solid_queue_scheduled_executions USING btree (scheduled_at, priority, job_id);


--
-- Name: index_solid_queue_failed_executions_on_job_id; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX index_solid_queue_failed_executions_on_job_id ON public.solid_queue_failed_executions USING btree (job_id);


--
-- Name: index_solid_queue_jobs_for_alerting; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_solid_queue_jobs_for_alerting ON public.solid_queue_jobs USING btree (scheduled_at, finished_at);


--
-- Name: index_solid_queue_jobs_for_filtering; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_solid_queue_jobs_for_filtering ON public.solid_queue_jobs USING btree (queue_name, finished_at);


--
-- Name: index_solid_queue_jobs_on_active_job_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_solid_queue_jobs_on_active_job_id ON public.solid_queue_jobs USING btree (active_job_id);


--
-- Name: index_solid_queue_jobs_on_class_name; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_solid_queue_jobs_on_class_name ON public.solid_queue_jobs USING btree (class_name);


--
-- Name: index_solid_queue_jobs_on_finished_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_solid_queue_jobs_on_finished_at ON public.solid_queue_jobs USING btree (finished_at);


--
-- Name: index_solid_queue_pauses_on_queue_name; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX index_solid_queue_pauses_on_queue_name ON public.solid_queue_pauses USING btree (queue_name);


--
-- Name: index_solid_queue_poll_all; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_solid_queue_poll_all ON public.solid_queue_ready_executions USING btree (priority, job_id);


--
-- Name: index_solid_queue_poll_by_queue; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_solid_queue_poll_by_queue ON public.solid_queue_ready_executions USING btree (queue_name, priority, job_id);


--
-- Name: index_solid_queue_processes_on_last_heartbeat_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_solid_queue_processes_on_last_heartbeat_at ON public.solid_queue_processes USING btree (last_heartbeat_at);


--
-- Name: index_solid_queue_processes_on_name_and_supervisor_id; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX index_solid_queue_processes_on_name_and_supervisor_id ON public.solid_queue_processes USING btree (name, supervisor_id);


--
-- Name: index_solid_queue_processes_on_supervisor_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_solid_queue_processes_on_supervisor_id ON public.solid_queue_processes USING btree (supervisor_id);


--
-- Name: index_solid_queue_ready_executions_on_job_id; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX index_solid_queue_ready_executions_on_job_id ON public.solid_queue_ready_executions USING btree (job_id);


--
-- Name: index_solid_queue_recurring_executions_on_job_id; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX index_solid_queue_recurring_executions_on_job_id ON public.solid_queue_recurring_executions USING btree (job_id);


--
-- Name: index_solid_queue_recurring_executions_on_task_key_and_run_at; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX index_solid_queue_recurring_executions_on_task_key_and_run_at ON public.solid_queue_recurring_executions USING btree (task_key, run_at);


--
-- Name: index_solid_queue_recurring_tasks_on_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX index_solid_queue_recurring_tasks_on_key ON public.solid_queue_recurring_tasks USING btree (key);


--
-- Name: index_solid_queue_recurring_tasks_on_static; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_solid_queue_recurring_tasks_on_static ON public.solid_queue_recurring_tasks USING btree (static);


--
-- Name: index_solid_queue_scheduled_executions_on_job_id; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX index_solid_queue_scheduled_executions_on_job_id ON public.solid_queue_scheduled_executions USING btree (job_id);


--
-- Name: index_solid_queue_semaphores_on_expires_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_solid_queue_semaphores_on_expires_at ON public.solid_queue_semaphores USING btree (expires_at);


--
-- Name: index_solid_queue_semaphores_on_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX index_solid_queue_semaphores_on_key ON public.solid_queue_semaphores USING btree (key);


--
-- Name: index_solid_queue_semaphores_on_key_and_value; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_solid_queue_semaphores_on_key_and_value ON public.solid_queue_semaphores USING btree (key, value);


--
-- Name: index_stripe_webhook_events_on_event_id; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX index_stripe_webhook_events_on_event_id ON public.stripe_webhook_events USING btree (event_id);


--
-- Name: index_user_audio_usages_on_audio_clip_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_user_audio_usages_on_audio_clip_id ON public.user_audio_usages USING btree (audio_clip_id);


--
-- Name: index_user_audio_usages_on_audio_type_and_last_used_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_user_audio_usages_on_audio_type_and_last_used_at ON public.user_audio_usages USING btree (audio_type, last_used_at);


--
-- Name: index_user_audio_usages_on_background_track_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_user_audio_usages_on_background_track_id ON public.user_audio_usages USING btree (background_track_id);


--
-- Name: index_user_audio_usages_on_identity; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX index_user_audio_usages_on_identity ON public.user_audio_usages USING btree (user_id, audio_type, asset_key);


--
-- Name: index_user_audio_usages_on_liturgical_text_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_user_audio_usages_on_liturgical_text_id ON public.user_audio_usages USING btree (liturgical_text_id);


--
-- Name: index_user_audio_usages_on_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_user_audio_usages_on_user_id ON public.user_audio_usages USING btree (user_id);


--
-- Name: index_user_audio_usages_on_user_id_and_last_used_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_user_audio_usages_on_user_id_and_last_used_at ON public.user_audio_usages USING btree (user_id, last_used_at);


--
-- Name: index_user_background_track_favorites_on_track_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_user_background_track_favorites_on_track_id ON public.user_background_track_favorites USING btree (track_id);


--
-- Name: index_user_background_track_favorites_on_user_and_track; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX index_user_background_track_favorites_on_user_and_track ON public.user_background_track_favorites USING btree (user_id, track_id);


--
-- Name: index_user_background_track_favorites_on_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_user_background_track_favorites_on_user_id ON public.user_background_track_favorites USING btree (user_id);


--
-- Name: index_user_favorites_on_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_user_favorites_on_user_id ON public.user_favorites USING btree (user_id);


--
-- Name: index_user_favorites_on_user_id_and_post_slug; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX index_user_favorites_on_user_id_and_post_slug ON public.user_favorites USING btree (user_id, post_slug);


--
-- Name: index_user_onboardings_on_bible_version_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_user_onboardings_on_bible_version_id ON public.user_onboardings USING btree (bible_version_id);


--
-- Name: index_user_onboardings_on_onboarding_completed; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_user_onboardings_on_onboarding_completed ON public.user_onboardings USING btree (onboarding_completed);


--
-- Name: index_user_onboardings_on_prayer_book_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_user_onboardings_on_prayer_book_id ON public.user_onboardings USING btree (prayer_book_id);


--
-- Name: index_user_onboardings_on_preferences; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_user_onboardings_on_preferences ON public.user_onboardings USING gin (preferences);


--
-- Name: index_user_onboardings_on_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX index_user_onboardings_on_user_id ON public.user_onboardings USING btree (user_id);


--
-- Name: index_users_on_country_code; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_users_on_country_code ON public.users USING btree (country_code);


--
-- Name: index_users_on_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_users_on_created_at ON public.users USING btree (created_at);


--
-- Name: index_users_on_email; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX index_users_on_email ON public.users USING btree (email);


--
-- Name: index_users_on_last_completed_office_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_users_on_last_completed_office_at ON public.users USING btree (last_completed_office_at);


--
-- Name: index_users_on_premium_expires_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_users_on_premium_expires_at ON public.users USING btree (premium_expires_at);


--
-- Name: index_users_on_provider_uid_unique; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX index_users_on_provider_uid_unique ON public.users USING btree (provider_uid);


--
-- Name: index_users_on_revenue_cat_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_users_on_revenue_cat_user_id ON public.users USING btree (revenue_cat_user_id);


--
-- Name: index_weekly_prayers_on_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_weekly_prayers_on_user_id ON public.weekly_prayers USING btree (user_id);


--
-- Name: weekly_prayers fk_rails_0208bae8c7; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.weekly_prayers
    ADD CONSTRAINT fk_rails_0208bae8c7 FOREIGN KEY (user_id) REFERENCES public.users(id);


--
-- Name: audio_clip_candidates fk_rails_0700cc31f1; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.audio_clip_candidates
    ADD CONSTRAINT fk_rails_0700cc31f1 FOREIGN KEY (audio_clip_id) REFERENCES public.audio_clips(id);


--
-- Name: custom_rosary_prayers fk_rails_08a455f370; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.custom_rosary_prayers
    ADD CONSTRAINT fk_rails_08a455f370 FOREIGN KEY (user_id) REFERENCES public.users(id);


--
-- Name: developer_checkout_sessions fk_rails_1ada41574a; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.developer_checkout_sessions
    ADD CONSTRAINT fk_rails_1ada41574a FOREIGN KEY (developer_id) REFERENCES public.developers(id);


--
-- Name: user_onboardings fk_rails_1b17bbc30d; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_onboardings
    ADD CONSTRAINT fk_rails_1b17bbc30d FOREIGN KEY (bible_version_id) REFERENCES public.bible_versions(id);


--
-- Name: completions fk_rails_1b5adf23d7; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.completions
    ADD CONSTRAINT fk_rails_1b5adf23d7 FOREIGN KEY (user_id) REFERENCES public.users(id);


--
-- Name: celebrations fk_rails_1db8353851; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.celebrations
    ADD CONSTRAINT fk_rails_1db8353851 FOREIGN KEY (prayer_book_id) REFERENCES public.prayer_books(id) ON DELETE RESTRICT;


--
-- Name: journals fk_rails_1f2015adde; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.journals
    ADD CONSTRAINT fk_rails_1f2015adde FOREIGN KEY (user_id) REFERENCES public.users(id);


--
-- Name: life_rule_exam_items fk_rails_237eaaeb78; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.life_rule_exam_items
    ADD CONSTRAINT fk_rails_237eaaeb78 FOREIGN KEY (life_rule_exam_id) REFERENCES public.life_rule_exams(id);


--
-- Name: user_favorites fk_rails_25ed4cb388; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_favorites
    ADD CONSTRAINT fk_rails_25ed4cb388 FOREIGN KEY (user_id) REFERENCES public.users(id);


--
-- Name: user_background_track_favorites fk_rails_29db69b31b; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_background_track_favorites
    ADD CONSTRAINT fk_rails_29db69b31b FOREIGN KEY (track_id) REFERENCES public.background_tracks(id) ON DELETE CASCADE;


--
-- Name: user_background_track_favorites fk_rails_2cdb39d8fe; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_background_track_favorites
    ADD CONSTRAINT fk_rails_2cdb39d8fe FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: solid_queue_recurring_executions fk_rails_318a5533ed; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.solid_queue_recurring_executions
    ADD CONSTRAINT fk_rails_318a5533ed FOREIGN KEY (job_id) REFERENCES public.solid_queue_jobs(id) ON DELETE CASCADE;


--
-- Name: user_audio_usages fk_rails_32868a212b; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_audio_usages
    ADD CONSTRAINT fk_rails_32868a212b FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: solid_queue_failed_executions fk_rails_39bbc7a631; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.solid_queue_failed_executions
    ADD CONSTRAINT fk_rails_39bbc7a631 FOREIGN KEY (job_id) REFERENCES public.solid_queue_jobs(id) ON DELETE CASCADE;


--
-- Name: audio_clip_candidates fk_rails_3cdc563908; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.audio_clip_candidates
    ADD CONSTRAINT fk_rails_3cdc563908 FOREIGN KEY (audio_operation_id) REFERENCES public.audio_operations(id);


--
-- Name: custom_rosary_steps fk_rails_3d24bda7bb; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.custom_rosary_steps
    ADD CONSTRAINT fk_rails_3d24bda7bb FOREIGN KEY (custom_rosary_block_id) REFERENCES public.custom_rosary_blocks(id) ON DELETE CASCADE;


--
-- Name: life_rule_steps fk_rails_3e9639fde9; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.life_rule_steps
    ADD CONSTRAINT fk_rails_3e9639fde9 FOREIGN KEY (life_rule_id) REFERENCES public.life_rules(id);


--
-- Name: developer_subscriptions fk_rails_4bfc75d979; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.developer_subscriptions
    ADD CONSTRAINT fk_rails_4bfc75d979 FOREIGN KEY (developer_id) REFERENCES public.developers(id);


--
-- Name: solid_queue_blocked_executions fk_rails_4cd34e2228; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.solid_queue_blocked_executions
    ADD CONSTRAINT fk_rails_4cd34e2228 FOREIGN KEY (job_id) REFERENCES public.solid_queue_jobs(id) ON DELETE CASCADE;


--
-- Name: collects fk_rails_4f65381b3e; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.collects
    ADD CONSTRAINT fk_rails_4f65381b3e FOREIGN KEY (celebration_id) REFERENCES public.celebrations(id);


--
-- Name: premium_subscription_events fk_rails_4fca1b1c3e; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.premium_subscription_events
    ADD CONSTRAINT fk_rails_4fca1b1c3e FOREIGN KEY (user_id) REFERENCES public.users(id);


--
-- Name: collects fk_rails_50543d1871; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.collects
    ADD CONSTRAINT fk_rails_50543d1871 FOREIGN KEY (prayer_book_id) REFERENCES public.prayer_books(id);


--
-- Name: user_audio_usages fk_rails_52fef05a44; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_audio_usages
    ADD CONSTRAINT fk_rails_52fef05a44 FOREIGN KEY (background_track_id) REFERENCES public.background_tracks(id) ON DELETE CASCADE;


--
-- Name: user_onboardings fk_rails_5325a2c6f5; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_onboardings
    ADD CONSTRAINT fk_rails_5325a2c6f5 FOREIGN KEY (prayer_book_id) REFERENCES public.prayer_books(id);


--
-- Name: user_onboardings fk_rails_66e66cf0e5; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_onboardings
    ADD CONSTRAINT fk_rails_66e66cf0e5 FOREIGN KEY (user_id) REFERENCES public.users(id);


--
-- Name: lectionary_readings fk_rails_6bdf22c99d; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.lectionary_readings
    ADD CONSTRAINT fk_rails_6bdf22c99d FOREIGN KEY (celebration_id) REFERENCES public.celebrations(id);


--
-- Name: background_track_categories fk_rails_723e6ecc7f; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.background_track_categories
    ADD CONSTRAINT fk_rails_723e6ecc7f FOREIGN KEY (track_id) REFERENCES public.background_tracks(id) ON DELETE CASCADE;


--
-- Name: background_track_assets fk_rails_74ea040835; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.background_track_assets
    ADD CONSTRAINT fk_rails_74ea040835 FOREIGN KEY (track_id) REFERENCES public.background_tracks(id) ON DELETE CASCADE;


--
-- Name: background_track_placements fk_rails_75a219b116; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.background_track_placements
    ADD CONSTRAINT fk_rails_75a219b116 FOREIGN KEY (track_id) REFERENCES public.background_tracks(id) ON DELETE CASCADE;


--
-- Name: life_rules fk_rails_805316429f; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.life_rules
    ADD CONSTRAINT fk_rails_805316429f FOREIGN KEY (user_id) REFERENCES public.users(id);


--
-- Name: user_audio_usages fk_rails_814deb6cb8; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_audio_usages
    ADD CONSTRAINT fk_rails_814deb6cb8 FOREIGN KEY (liturgical_text_id) REFERENCES public.liturgical_texts(id) ON DELETE CASCADE;


--
-- Name: solid_queue_ready_executions fk_rails_81fcbd66af; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.solid_queue_ready_executions
    ADD CONSTRAINT fk_rails_81fcbd66af FOREIGN KEY (job_id) REFERENCES public.solid_queue_jobs(id) ON DELETE CASCADE;


--
-- Name: life_rule_exams fk_rails_87fd6348f3; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.life_rule_exams
    ADD CONSTRAINT fk_rails_87fd6348f3 FOREIGN KEY (user_id) REFERENCES public.users(id);


--
-- Name: collects fk_rails_8a18f1d74f; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.collects
    ADD CONSTRAINT fk_rails_8a18f1d74f FOREIGN KEY (season_id) REFERENCES public.liturgical_seasons(id);


--
-- Name: custom_rosary_blocks fk_rails_8eabf745f8; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.custom_rosary_blocks
    ADD CONSTRAINT fk_rails_8eabf745f8 FOREIGN KEY (custom_rosary_prayer_id) REFERENCES public.custom_rosary_prayers(id) ON DELETE CASCADE;


--
-- Name: feature_flags fk_rails_97bde8353d; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.feature_flags
    ADD CONSTRAINT fk_rails_97bde8353d FOREIGN KEY (user_id) REFERENCES public.users(id);


--
-- Name: active_storage_variant_records fk_rails_993965df05; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.active_storage_variant_records
    ADD CONSTRAINT fk_rails_993965df05 FOREIGN KEY (blob_id) REFERENCES public.active_storage_blobs(id);


--
-- Name: solid_queue_claimed_executions fk_rails_9cfe4d4944; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.solid_queue_claimed_executions
    ADD CONSTRAINT fk_rails_9cfe4d4944 FOREIGN KEY (job_id) REFERENCES public.solid_queue_jobs(id) ON DELETE CASCADE;


--
-- Name: preference_categories fk_rails_9f5ecb6be7; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.preference_categories
    ADD CONSTRAINT fk_rails_9f5ecb6be7 FOREIGN KEY (prayer_book_id) REFERENCES public.prayer_books(id);


--
-- Name: audio_clip_usages fk_rails_a179bcf675; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.audio_clip_usages
    ADD CONSTRAINT fk_rails_a179bcf675 FOREIGN KEY (audio_clip_id) REFERENCES public.audio_clips(id);


--
-- Name: psalm_cycles fk_rails_a3e51f2cf5; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.psalm_cycles
    ADD CONSTRAINT fk_rails_a3e51f2cf5 FOREIGN KEY (prayer_book_id) REFERENCES public.prayer_books(id);


--
-- Name: background_track_categories fk_rails_a3f1e2508f; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.background_track_categories
    ADD CONSTRAINT fk_rails_a3f1e2508f FOREIGN KEY (category_id) REFERENCES public.background_categories(id) ON DELETE CASCADE;


--
-- Name: liturgical_texts fk_rails_a76e71a74d; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.liturgical_texts
    ADD CONSTRAINT fk_rails_a76e71a74d FOREIGN KEY (prayer_book_id) REFERENCES public.prayer_books(id) ON DELETE RESTRICT;


--
-- Name: preference_definitions fk_rails_b14ec1fa1e; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.preference_definitions
    ADD CONSTRAINT fk_rails_b14ec1fa1e FOREIGN KEY (preference_category_id) REFERENCES public.preference_categories(id);


--
-- Name: api_keys fk_rails_b804a65aba; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.api_keys
    ADD CONSTRAINT fk_rails_b804a65aba FOREIGN KEY (developer_id) REFERENCES public.developers(id);


--
-- Name: completions fk_rails_b80edb967f; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.completions
    ADD CONSTRAINT fk_rails_b80edb967f FOREIGN KEY (prayer_book_id) REFERENCES public.prayer_books(id);


--
-- Name: shared_offices fk_rails_bd9ba973d1; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.shared_offices
    ADD CONSTRAINT fk_rails_bd9ba973d1 FOREIGN KEY (user_id) REFERENCES public.users(id);


--
-- Name: life_rules fk_rails_be3515aaaf; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.life_rules
    ADD CONSTRAINT fk_rails_be3515aaaf FOREIGN KEY (original_life_rule_id) REFERENCES public.life_rules(id);


--
-- Name: active_storage_attachments fk_rails_c3b3935057; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.active_storage_attachments
    ADD CONSTRAINT fk_rails_c3b3935057 FOREIGN KEY (blob_id) REFERENCES public.active_storage_blobs(id);


--
-- Name: solid_queue_scheduled_executions fk_rails_c4316f352d; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.solid_queue_scheduled_executions
    ADD CONSTRAINT fk_rails_c4316f352d FOREIGN KEY (job_id) REFERENCES public.solid_queue_jobs(id) ON DELETE CASCADE;


--
-- Name: fcm_tokens fk_rails_c7f9e8f554; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.fcm_tokens
    ADD CONSTRAINT fk_rails_c7f9e8f554 FOREIGN KEY (user_id) REFERENCES public.users(id);


--
-- Name: prayer_requests fk_rails_cf4b95424f; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.prayer_requests
    ADD CONSTRAINT fk_rails_cf4b95424f FOREIGN KEY (user_id) REFERENCES public.users(id);


--
-- Name: user_audio_usages fk_rails_e3ac587986; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_audio_usages
    ADD CONSTRAINT fk_rails_e3ac587986 FOREIGN KEY (audio_clip_id) REFERENCES public.audio_clips(id) ON DELETE CASCADE;


--
-- Name: lectionary_readings fk_rails_e7e06a59c8; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.lectionary_readings
    ADD CONSTRAINT fk_rails_e7e06a59c8 FOREIGN KEY (prayer_book_id) REFERENCES public.prayer_books(id);


--
-- Name: api_key_usage_logs fk_rails_f5987cbbbc; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.api_key_usage_logs
    ADD CONSTRAINT fk_rails_f5987cbbbc FOREIGN KEY (api_key_id) REFERENCES public.api_keys(id);


--
-- Name: psalms fk_rails_fc7b7b9f6a; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.psalms
    ADD CONSTRAINT fk_rails_fc7b7b9f6a FOREIGN KEY (prayer_book_id) REFERENCES public.prayer_books(id) ON DELETE RESTRICT;


--
-- Name: notification_logs fk_rails_fc99b64783; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.notification_logs
    ADD CONSTRAINT fk_rails_fc99b64783 FOREIGN KEY (user_id) REFERENCES public.users(id);


--
-- PostgreSQL database dump complete
--



SET search_path = public;
INSERT INTO schema_migrations (version) VALUES
('20251116022111'),
('20251116022129'),
('20251116022139'),
('20251116022149'),
('20251116022159'),
('20251116025902'),
('20251116181500'),
('20251122032342'),
('20251122032353'),
('20251122032405'),
('20251122032415'),
('20251122222906'),
('20251122222907'),
('20251123000001'),
('20251123000002'),
('20251124184735'),
('20251124185205'),
('20251124185234'),
('20251124185235'),
('20251124185236'),
('20251124185237'),
('20251124200629'),
('20251125154646'),
('20251125154717'),
('20251125165648'),
('20251125191610'),
('20251128000001'),
('20251128000002'),
('20251128000003'),
('20251128000004'),
('20251129032806'),
('20251201210620'),
('20251202000001'),
('20251202000002'),
('20251202000003'),
('20251202000004'),
('20251202000005'),
('20251202000006'),
('20251203181513'),
('20251204174043'),
('20251204184200'),
('20251204200000'),
('20251204200100'),
('20251210014700'),
('20251213043607'),
('20251213144621'),
('20251213144717'),
('20251213145128'),
('20251218194058'),
('20251219171554'),
('20251219235257'),
('20251223183708'),
('20251229200000'),
('20251229210000'),
('20251229215548'),
('20251230175957'),
('20260108205944'),
('20260120000000'),
('20260121001922'),
('20260123201913'),
('20260206131126'),
('20260206135412'),
('20260206152019'),
('20260208120000'),
('20260209201032'),
('20260212035315'),
('20260212035332'),
('20260214014111'),
('20260214022148'),
('20260214024327'),
('20260214024328'),
('20260217150000'),
('20260217160000'),
('20260217170000'),
('20260223130000'),
('20260223234506'),
('20260313180657'),
('20260708150000'),
('20260714120000'),
('20260722191208'),
('20260725120000'),
('20260728144019'),
('20260730180000'),
('20260801150000'),
('20260801153000'),
('20260801160000'),
('20260801161000'),
('20260802120000'),
('20260803101000'),
('20260803102000'),
('20260803103000'),
('20260803104000'),
('20260803105000'),
('20260803106000'),
('20260808120000'),
('20260809120000'),
('20260810120000'),
('20260810120001'),
('20260810120002'),
('20260810130000'),
('20260810130001'),
('20260810130002'),
('20260814100000'),
('20260814120000'),
('20260818140000'),
('20260820100000'),
('20260820110000'),
('20260821100000'),
('20260821110000'),
('20260822120000'),
('20260823120000'),
('20260826120000'),
('20260828120000'),
('20260831120000'),
('20260902010000'),
('20260903120000'),
('20260915120000'),
('20260916130000'),
('20260917130000'),
('20260917130100'),
('20260917130200'),
('20260917130300'),
('20260917140000'),
('20260919090000'),
('20260919170000'),
('20260919180000'),
('20260920193000'),
('20260921100000'),
('20260922120000'),
('20260922140000'),
('20260922160000')
;
