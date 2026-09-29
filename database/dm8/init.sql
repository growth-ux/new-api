-- DM8 init: LDW_AI_AGG

CREATE TABLE "LDW_AI_AGG"."channels" ("id" BIGINT IDENTITY(1,1),"type" BIGINT DEFAULT 0,"key" VARCHAR(8188 CHAR) NOT NULL,"open_ai_organization" VARCHAR(8188 CHAR),"test_model" VARCHAR(8188 CHAR),"status" BIGINT DEFAULT 1,"name" VARCHAR(255 CHAR),"weight" BIGINT DEFAULT 0,"created_time" BIGINT,"test_time" BIGINT,"response_time" BIGINT,"base_url" VARCHAR(255 CHAR) DEFAULT '',"other" VARCHAR(8188 CHAR),"balance" DOUBLE,"balance_updated_time" BIGINT,"models" VARCHAR(8188 CHAR),"group" varchar(64) DEFAULT 'default',"used_quota" BIGINT DEFAULT 0,"model_mapping" text,"status_code_mapping" varchar(1024) DEFAULT '',"priority" BIGINT DEFAULT 0,"auto_ban" BIGINT DEFAULT 1,"other_info" VARCHAR(8188 CHAR),"tag" VARCHAR(255 CHAR),"setting" text,"param_override" text,"header_override" text,"remark" varchar(255),"channel_info" CLOB,"settings" VARCHAR(8188 CHAR),PRIMARY KEY ("id"));

CREATE INDEX "idx_channels_tag" ON "LDW_AI_AGG"."channels"("tag");

CREATE INDEX "idx_channels_name" ON "LDW_AI_AGG"."channels"("name");

CREATE TABLE "LDW_AI_AGG"."tokens" ("id" BIGINT IDENTITY(1,1),"user_id" BIGINT,"key" varchar(128),"status" BIGINT DEFAULT 1,"name" VARCHAR(255 CHAR),"created_time" BIGINT,"accessed_time" BIGINT,"expired_time" BIGINT DEFAULT -1,"remain_quota" BIGINT DEFAULT 0,"unlimited_quota" BIT,"model_limits_enabled" BIT,"model_limits" text,"allow_ips" VARCHAR(255 CHAR) DEFAULT '',"used_quota" BIGINT DEFAULT 0,"group" VARCHAR(255 CHAR) DEFAULT '',"cross_group_retry" BIT,"auto_groups" text,"deleted_at" DATETIME WITH TIME ZONE,PRIMARY KEY ("id"));

CREATE INDEX "idx_tokens_deleted_at" ON "LDW_AI_AGG"."tokens"("deleted_at");

CREATE INDEX "idx_tokens_name" ON "LDW_AI_AGG"."tokens"("name");

CREATE UNIQUE INDEX "idx_tokens_key" ON "LDW_AI_AGG"."tokens"("key");

CREATE INDEX "idx_tokens_user_id" ON "LDW_AI_AGG"."tokens"("user_id");

CREATE TABLE "LDW_AI_AGG"."users" ("id" BIGINT IDENTITY(1,1),"username" VARCHAR(255 CHAR),"password" VARCHAR(8188 CHAR) NOT NULL,"display_name" VARCHAR(255 CHAR),"role" BIGINT DEFAULT 1,"status" BIGINT DEFAULT 1,"email" VARCHAR(255 CHAR),"github_id" VARCHAR(255 CHAR),"discord_id" VARCHAR(255 CHAR),"oidc_id" VARCHAR(255 CHAR),"wechat_id" VARCHAR(255 CHAR),"telegram_id" VARCHAR(255 CHAR),"access_token" char(32),"access_token_created_at" bigint,"quota" BIGINT DEFAULT 0,"used_quota" BIGINT DEFAULT 0,"request_count" BIGINT DEFAULT 0,"group" varchar(64) DEFAULT 'default',"aff_code" varchar(32),"aff_count" BIGINT DEFAULT 0,"aff_quota" BIGINT DEFAULT 0,"aff_history" BIGINT DEFAULT 0,"inviter_id" BIGINT,"deleted_at" DATETIME WITH TIME ZONE,"linux_do_id" VARCHAR(255 CHAR),"setting" text,"remark" varchar(255),"stripe_customer" varchar(64),"created_at" BIGINT,"last_login_at" BIGINT DEFAULT 0,"auth_version" bigint NOT NULL DEFAULT 1,PRIMARY KEY ("id"),CONSTRAINT "uni_users_username" UNIQUE ("username"));

CREATE INDEX "idx_users_git_hub_id" ON "LDW_AI_AGG"."users"("github_id");

CREATE INDEX "idx_users_email" ON "LDW_AI_AGG"."users"("email");

CREATE INDEX "idx_users_display_name" ON "LDW_AI_AGG"."users"("display_name");

CREATE INDEX "idx_users_deleted_at" ON "LDW_AI_AGG"."users"("deleted_at");

CREATE INDEX "idx_users_inviter_id" ON "LDW_AI_AGG"."users"("inviter_id");

CREATE INDEX "idx_users_we_chat_id" ON "LDW_AI_AGG"."users"("wechat_id");

CREATE INDEX "idx_users_oidc_id" ON "LDW_AI_AGG"."users"("oidc_id");

CREATE INDEX "idx_users_discord_id" ON "LDW_AI_AGG"."users"("discord_id");

CREATE INDEX "idx_users_username" ON "LDW_AI_AGG"."users"("username");

CREATE INDEX "idx_users_stripe_customer" ON "LDW_AI_AGG"."users"("stripe_customer");

CREATE INDEX "idx_users_linux_do_id" ON "LDW_AI_AGG"."users"("linux_do_id");

CREATE UNIQUE INDEX "idx_users_aff_code" ON "LDW_AI_AGG"."users"("aff_code");

CREATE UNIQUE INDEX "idx_users_access_token" ON "LDW_AI_AGG"."users"("access_token");

CREATE INDEX "idx_users_telegram_id" ON "LDW_AI_AGG"."users"("telegram_id");

CREATE TABLE "LDW_AI_AGG"."user_sessions" ("sid" varchar(64),"user_id" BIGINT NOT NULL,"version" bigint NOT NULL DEFAULT 1,"user_auth_version" bigint NOT NULL,"status" varchar(16) NOT NULL,"refresh_hash" char(64) NOT NULL,"previous_refresh_hash" varchar(64),"previous_valid_until" bigint NOT NULL DEFAULT 0,"login_method" varchar(32) NOT NULL,"ip" varchar(64),"user_agent" text,"created_at" BIGINT,"last_active_at" bigint NOT NULL,"expires_at" bigint NOT NULL,"revoked_at" bigint NOT NULL DEFAULT 0,"revoked_reason" varchar(64),PRIMARY KEY ("sid"));

CREATE INDEX "idx_user_sessions_expires_at" ON "LDW_AI_AGG"."user_sessions"("expires_at");

CREATE INDEX "idx_user_sessions_status_revoked" ON "LDW_AI_AGG"."user_sessions"("status","revoked_at");

CREATE INDEX "idx_user_sessions_user_created" ON "LDW_AI_AGG"."user_sessions"("user_id","created_at");

CREATE INDEX "idx_user_sessions_user_status_expiry" ON "LDW_AI_AGG"."user_sessions"("user_id","status","expires_at");

CREATE TABLE "LDW_AI_AGG"."auth_flows" ("id" BIGINT IDENTITY(1,1),"token_hash" char(64) NOT NULL,"purpose" varchar(32) NOT NULL,"provider" varchar(64),"intent" varchar(16),"user_id" BIGINT,"session_id" varchar(64),"payload" text,"created_at" DATETIME WITH TIME ZONE,"expires_at" DATETIME WITH TIME ZONE NOT NULL,"consumed_at" DATETIME WITH TIME ZONE,PRIMARY KEY ("id"));

CREATE UNIQUE INDEX "idx_auth_flows_token_hash" ON "LDW_AI_AGG"."auth_flows"("token_hash");

CREATE INDEX "idx_auth_flows_consumed_at" ON "LDW_AI_AGG"."auth_flows"("consumed_at");

CREATE INDEX "idx_auth_flows_session_id" ON "LDW_AI_AGG"."auth_flows"("session_id");

CREATE INDEX "idx_auth_flows_user_id" ON "LDW_AI_AGG"."auth_flows"("user_id");

CREATE INDEX "idx_auth_flow_purpose_expiry" ON "LDW_AI_AGG"."auth_flows"("purpose","expires_at");

CREATE TABLE "LDW_AI_AGG"."external_identity_claims" ("id" BIGINT IDENTITY(1,1),"provider" varchar(32) NOT NULL,"subject" varchar(128) NOT NULL,"user_id" BIGINT NOT NULL,"created_at" DATETIME WITH TIME ZONE,PRIMARY KEY ("id"));

CREATE UNIQUE INDEX "idx_external_identity_subject" ON "LDW_AI_AGG"."external_identity_claims"("provider","subject");

CREATE INDEX "idx_external_identity_claims_user_id" ON "LDW_AI_AGG"."external_identity_claims"("user_id");

CREATE UNIQUE INDEX "idx_external_identity_user" ON "LDW_AI_AGG"."external_identity_claims"("provider","user_id");

CREATE TABLE "LDW_AI_AGG"."passkey_credentials" ("id" BIGINT IDENTITY(1,1),"user_id" BIGINT NOT NULL,"rp_id" varchar(253),"credential_id" varchar(512) NOT NULL,"public_key" text NOT NULL,"attestation_type" varchar(255),"aa_guid" varchar(512),"sign_count" INT DEFAULT 0,"clone_warning" BIT,"user_present" BIT,"user_verified" BIT,"backup_eligible" BIT,"backup_state" BIT,"transports" text,"attachment" varchar(32),"last_used_at" DATETIME WITH TIME ZONE,"created_at" DATETIME WITH TIME ZONE,"updated_at" DATETIME WITH TIME ZONE,"deleted_at" DATETIME WITH TIME ZONE,PRIMARY KEY ("id"));

CREATE INDEX "idx_passkey_credentials_deleted_at" ON "LDW_AI_AGG"."passkey_credentials"("deleted_at");

CREATE UNIQUE INDEX "idx_passkey_credentials_credential_id" ON "LDW_AI_AGG"."passkey_credentials"("credential_id");

CREATE UNIQUE INDEX "idx_passkey_credentials_user_id" ON "LDW_AI_AGG"."passkey_credentials"("user_id");

CREATE TABLE "LDW_AI_AGG"."options" ("key" VARCHAR(255 CHAR),"value" VARCHAR(8188 CHAR),PRIMARY KEY ("key"));

CREATE TABLE "LDW_AI_AGG"."login_encryption_keys" ("id" BIGINT IDENTITY(1,1),"slot" varchar(32) NOT NULL,"private_key_pem" text NOT NULL,PRIMARY KEY ("id"));

CREATE UNIQUE INDEX "idx_login_encryption_keys_slot" ON "LDW_AI_AGG"."login_encryption_keys"("slot");

CREATE TABLE "LDW_AI_AGG"."redemptions" ("id" BIGINT IDENTITY(1,1),"user_id" BIGINT,"key" char(32),"status" BIGINT DEFAULT 1,"name" VARCHAR(255 CHAR),"quota" BIGINT DEFAULT 100,"created_time" BIGINT,"redeemed_time" BIGINT,"used_user_id" BIGINT,"deleted_at" DATETIME WITH TIME ZONE,"expired_time" BIGINT,PRIMARY KEY ("id"));

CREATE INDEX "idx_redemptions_deleted_at" ON "LDW_AI_AGG"."redemptions"("deleted_at");

CREATE INDEX "idx_redemptions_name" ON "LDW_AI_AGG"."redemptions"("name");

CREATE UNIQUE INDEX "idx_redemptions_key" ON "LDW_AI_AGG"."redemptions"("key");

CREATE TABLE "LDW_AI_AGG"."abilities" ("group" varchar(64),"model" varchar(255),"channel_id" BIGINT,"enabled" BIT,"priority" BIGINT DEFAULT 0,"weight" BIGINT DEFAULT 0,"tag" VARCHAR(255 CHAR),PRIMARY KEY ("group","model","channel_id"));

CREATE INDEX "idx_abilities_tag" ON "LDW_AI_AGG"."abilities"("tag");

CREATE INDEX "idx_abilities_weight" ON "LDW_AI_AGG"."abilities"("weight");

CREATE INDEX "idx_abilities_priority" ON "LDW_AI_AGG"."abilities"("priority");

CREATE INDEX "idx_abilities_channel_id" ON "LDW_AI_AGG"."abilities"("channel_id");

CREATE TABLE "LDW_AI_AGG"."midjourneys" ("id" BIGINT IDENTITY(1,1),"code" BIGINT,"user_id" BIGINT,"action" varchar(40),"mj_id" VARCHAR(255 CHAR),"prompt" VARCHAR(8188 CHAR),"prompt_en" VARCHAR(8188 CHAR),"description" VARCHAR(8188 CHAR),"state" VARCHAR(8188 CHAR),"submit_time" BIGINT,"start_time" BIGINT,"finish_time" BIGINT,"image_url" VARCHAR(8188 CHAR),"video_url" VARCHAR(8188 CHAR),"video_urls" VARCHAR(8188 CHAR),"status" varchar(20),"progress" varchar(30),"fail_reason" VARCHAR(8188 CHAR),"channel_id" BIGINT,"quota" BIGINT,"buttons" VARCHAR(8188 CHAR),"properties" VARCHAR(8188 CHAR),"token_id" BIGINT DEFAULT 0,"billing_channel_id" BIGINT DEFAULT 0,PRIMARY KEY ("id"));

CREATE INDEX "idx_midjourneys_start_time" ON "LDW_AI_AGG"."midjourneys"("start_time");

CREATE INDEX "idx_midjourneys_submit_time" ON "LDW_AI_AGG"."midjourneys"("submit_time");

CREATE INDEX "idx_midjourneys_mj_id" ON "LDW_AI_AGG"."midjourneys"("mj_id");

CREATE INDEX "idx_midjourneys_action" ON "LDW_AI_AGG"."midjourneys"("action");

CREATE INDEX "idx_midjourneys_user_id" ON "LDW_AI_AGG"."midjourneys"("user_id");

CREATE INDEX "idx_midjourneys_progress" ON "LDW_AI_AGG"."midjourneys"("progress");

CREATE INDEX "idx_midjourneys_status" ON "LDW_AI_AGG"."midjourneys"("status");

CREATE INDEX "idx_midjourneys_finish_time" ON "LDW_AI_AGG"."midjourneys"("finish_time");

CREATE TABLE "LDW_AI_AGG"."top_ups" ("id" BIGINT IDENTITY(1,1),"user_id" BIGINT,"amount" BIGINT,"money" DOUBLE,"trade_no" varchar(255),"payment_method" varchar(50),"payment_provider" varchar(50) DEFAULT '',"create_time" BIGINT,"complete_time" BIGINT,"status" VARCHAR(8188 CHAR),PRIMARY KEY ("id"),CONSTRAINT "uni_top_ups_trade_no" UNIQUE ("trade_no"));

CREATE INDEX "idx_top_ups_trade_no" ON "LDW_AI_AGG"."top_ups"("trade_no");

CREATE INDEX "idx_top_ups_user_id" ON "LDW_AI_AGG"."top_ups"("user_id");

CREATE TABLE "LDW_AI_AGG"."quota_data" ("id" BIGINT IDENTITY(1,1),"user_id" BIGINT,"username" VARCHAR(64 CHAR) DEFAULT '',"model_name" VARCHAR(64 CHAR) DEFAULT '',"created_at" BIGINT,"use_group" VARCHAR(64 CHAR) DEFAULT '',"token_id" BIGINT DEFAULT 0,"channel_id" BIGINT DEFAULT 0,"node_name" VARCHAR(64 CHAR) DEFAULT '',"token_used" BIGINT DEFAULT 0,"count" BIGINT DEFAULT 0,"quota" BIGINT DEFAULT 0,PRIMARY KEY ("id"));

CREATE INDEX "idx_qdt_created_at" ON "LDW_AI_AGG"."quota_data"("created_at");

CREATE INDEX "idx_qdt_model_user_name" ON "LDW_AI_AGG"."quota_data"("model_name","username");

CREATE INDEX "idx_quota_data_user_id" ON "LDW_AI_AGG"."quota_data"("user_id");

CREATE INDEX "idx_quota_data_node_name" ON "LDW_AI_AGG"."quota_data"("node_name");

CREATE INDEX "idx_quota_data_channel_id" ON "LDW_AI_AGG"."quota_data"("channel_id");

CREATE INDEX "idx_quota_data_token_id" ON "LDW_AI_AGG"."quota_data"("token_id");

CREATE INDEX "idx_quota_data_use_group" ON "LDW_AI_AGG"."quota_data"("use_group");

CREATE TABLE "LDW_AI_AGG"."tasks" ("id" BIGINT IDENTITY(1,1),"created_at" BIGINT,"updated_at" BIGINT,"task_id" varchar(191),"platform" varchar(30),"user_id" BIGINT,"group" varchar(50),"channel_id" BIGINT,"quota" BIGINT,"action" varchar(40),"status" varchar(20),"fail_reason" VARCHAR(8188 CHAR),"submit_time" BIGINT,"start_time" BIGINT,"finish_time" BIGINT,"progress" varchar(20),"properties" CLOB,"private_data" CLOB,"data" CLOB,PRIMARY KEY ("id"));

CREATE INDEX "idx_tasks_finish_time" ON "LDW_AI_AGG"."tasks"("finish_time");

CREATE INDEX "idx_tasks_start_time" ON "LDW_AI_AGG"."tasks"("start_time");

CREATE INDEX "idx_tasks_channel_id" ON "LDW_AI_AGG"."tasks"("channel_id");

CREATE INDEX "idx_tasks_task_id" ON "LDW_AI_AGG"."tasks"("task_id");

CREATE INDEX "idx_tasks_progress" ON "LDW_AI_AGG"."tasks"("progress");

CREATE INDEX "idx_tasks_submit_time" ON "LDW_AI_AGG"."tasks"("submit_time");

CREATE INDEX "idx_tasks_status" ON "LDW_AI_AGG"."tasks"("status");

CREATE INDEX "idx_tasks_action" ON "LDW_AI_AGG"."tasks"("action");

CREATE INDEX "idx_tasks_user_id" ON "LDW_AI_AGG"."tasks"("user_id");

CREATE INDEX "idx_tasks_platform" ON "LDW_AI_AGG"."tasks"("platform");

CREATE INDEX "idx_tasks_created_at" ON "LDW_AI_AGG"."tasks"("created_at");

CREATE TABLE "LDW_AI_AGG"."task_plugins" ("id" BIGINT IDENTITY(1,1),"key" VARCHAR(128 CHAR) NOT NULL,"api_version" BIGINT NOT NULL,"version" VARCHAR(64 CHAR) NOT NULL,"source" text NOT NULL,"source_hash" VARCHAR(64 CHAR) NOT NULL,"icon" text,"enabled" BIT NOT NULL,"active" BIT NOT NULL,"created_at" BIGINT NOT NULL,"remark" text,PRIMARY KEY ("id"));

CREATE INDEX "idx_task_plugins_active" ON "LDW_AI_AGG"."task_plugins"("active");

CREATE UNIQUE INDEX "uk_task_plugin_key_version" ON "LDW_AI_AGG"."task_plugins"("key","version");

CREATE TABLE "LDW_AI_AGG"."models" ("id" BIGINT IDENTITY(1,1),"model_name" VARCHAR(128 CHAR) NOT NULL,"description" text,"icon" varchar(128),"tags" varchar(255),"vendor_id" BIGINT,"endpoints" text,"status" BIGINT DEFAULT 1,"sync_official" BIGINT DEFAULT 1,"created_time" BIGINT,"updated_time" BIGINT,"deleted_at" DATETIME WITH TIME ZONE,"name_rule" BIGINT DEFAULT 0,PRIMARY KEY ("id"));

CREATE INDEX "idx_models_deleted_at" ON "LDW_AI_AGG"."models"("deleted_at");

CREATE INDEX "idx_models_vendor_id" ON "LDW_AI_AGG"."models"("vendor_id");

CREATE UNIQUE INDEX "uk_model_name_delete_at" ON "LDW_AI_AGG"."models"("model_name","deleted_at");

CREATE TABLE "LDW_AI_AGG"."vendors" ("id" BIGINT IDENTITY(1,1),"name" VARCHAR(128 CHAR) NOT NULL,"description" text,"icon" varchar(128),"status" BIGINT DEFAULT 1,"created_time" BIGINT,"updated_time" BIGINT,"deleted_at" DATETIME WITH TIME ZONE,PRIMARY KEY ("id"));

CREATE INDEX "idx_vendors_deleted_at" ON "LDW_AI_AGG"."vendors"("deleted_at");

CREATE UNIQUE INDEX "uk_vendor_name_delete_at" ON "LDW_AI_AGG"."vendors"("name","deleted_at");

CREATE TABLE "LDW_AI_AGG"."prefill_groups" ("id" BIGINT IDENTITY(1,1),"name" VARCHAR(64 CHAR) NOT NULL,"type" VARCHAR(32 CHAR) NOT NULL,"items" CLOB,"description" varchar(255),"created_time" BIGINT,"updated_time" BIGINT,"deleted_at" DATETIME WITH TIME ZONE,PRIMARY KEY ("id"));

CREATE INDEX "idx_prefill_groups_deleted_at" ON "LDW_AI_AGG"."prefill_groups"("deleted_at");

CREATE INDEX "idx_prefill_groups_type" ON "LDW_AI_AGG"."prefill_groups"("type");

CREATE TABLE "LDW_AI_AGG"."setups" ("id" BIGINT IDENTITY(1,1),"version" varchar(50) NOT NULL,"initialized_at" bigint NOT NULL,PRIMARY KEY ("id"));

CREATE TABLE "LDW_AI_AGG"."two_fas" ("id" BIGINT IDENTITY(1,1),"user_id" BIGINT NOT NULL,"secret" varchar(255) NOT NULL,"is_enabled" BIT,"failed_attempts" BIGINT DEFAULT 0,"locked_until" DATETIME WITH TIME ZONE,"last_used_at" DATETIME WITH TIME ZONE,"created_at" DATETIME WITH TIME ZONE,"updated_at" DATETIME WITH TIME ZONE,"deleted_at" DATETIME WITH TIME ZONE,PRIMARY KEY ("id"),CONSTRAINT "uni_two_fas_user_id" UNIQUE ("user_id"));

CREATE INDEX "idx_two_fas_deleted_at" ON "LDW_AI_AGG"."two_fas"("deleted_at");

CREATE INDEX "idx_two_fas_user_id" ON "LDW_AI_AGG"."two_fas"("user_id");

CREATE TABLE "LDW_AI_AGG"."two_fa_backup_codes" ("id" BIGINT IDENTITY(1,1),"user_id" BIGINT NOT NULL,"code_hash" varchar(255) NOT NULL,"is_used" BIT,"used_at" DATETIME WITH TIME ZONE,"created_at" DATETIME WITH TIME ZONE,"deleted_at" DATETIME WITH TIME ZONE,PRIMARY KEY ("id"));

CREATE INDEX "idx_two_fa_backup_codes_user_id" ON "LDW_AI_AGG"."two_fa_backup_codes"("user_id");

CREATE INDEX "idx_two_fa_backup_codes_deleted_at" ON "LDW_AI_AGG"."two_fa_backup_codes"("deleted_at");

CREATE TABLE "LDW_AI_AGG"."checkins" ("id" BIGINT IDENTITY(1,1),"user_id" BIGINT NOT NULL,"checkin_date" varchar(10) NOT NULL,"quota_awarded" BIGINT NOT NULL,"created_at" BIGINT,PRIMARY KEY ("id"));

CREATE UNIQUE INDEX "idx_user_checkin_date" ON "LDW_AI_AGG"."checkins"("user_id","checkin_date");

CREATE TABLE "LDW_AI_AGG"."subscription_orders" ("id" BIGINT IDENTITY(1,1),"user_id" BIGINT,"plan_id" BIGINT,"money" DOUBLE,"trade_no" varchar(255),"payment_method" varchar(50),"payment_provider" varchar(50) DEFAULT '',"status" VARCHAR(8188 CHAR),"create_time" BIGINT,"complete_time" BIGINT,"provider_payload" text,PRIMARY KEY ("id"),CONSTRAINT "uni_subscription_orders_trade_no" UNIQUE ("trade_no"));

CREATE INDEX "idx_subscription_orders_user_id" ON "LDW_AI_AGG"."subscription_orders"("user_id");

CREATE INDEX "idx_subscription_orders_trade_no" ON "LDW_AI_AGG"."subscription_orders"("trade_no");

CREATE INDEX "idx_subscription_orders_plan_id" ON "LDW_AI_AGG"."subscription_orders"("plan_id");

CREATE TABLE "LDW_AI_AGG"."user_subscriptions" ("id" BIGINT IDENTITY(1,1),"user_id" BIGINT,"plan_id" BIGINT,"amount_total" bigint NOT NULL DEFAULT 0,"amount_used" bigint NOT NULL DEFAULT 0,"start_time" BIGINT,"end_time" BIGINT,"status" varchar(32),"source" varchar(32) DEFAULT 'order',"last_reset_time" bigint DEFAULT 0,"next_reset_time" bigint DEFAULT 0,"upgrade_group" varchar(64) DEFAULT '',"prev_user_group" varchar(64) DEFAULT '',"downgrade_group" varchar(64) DEFAULT '',"allow_wallet_overflow" BIT,"created_at" BIGINT,"updated_at" BIGINT,PRIMARY KEY ("id"));

CREATE INDEX "idx_user_subscriptions_status" ON "LDW_AI_AGG"."user_subscriptions"("status");

CREATE INDEX "idx_user_subscriptions_end_time" ON "LDW_AI_AGG"."user_subscriptions"("end_time");

CREATE INDEX "idx_user_subscriptions_plan_id" ON "LDW_AI_AGG"."user_subscriptions"("plan_id");

CREATE INDEX "idx_user_sub_active" ON "LDW_AI_AGG"."user_subscriptions"("user_id","status","end_time");

CREATE INDEX "idx_user_subscriptions_user_id" ON "LDW_AI_AGG"."user_subscriptions"("user_id");

CREATE INDEX "idx_user_subscriptions_next_reset_time" ON "LDW_AI_AGG"."user_subscriptions"("next_reset_time");

CREATE TABLE "LDW_AI_AGG"."subscription_pre_consume_records" ("id" BIGINT IDENTITY(1,1),"request_id" varchar(64),"user_id" BIGINT,"user_subscription_id" BIGINT,"pre_consumed" bigint NOT NULL DEFAULT 0,"status" varchar(32),"created_at" BIGINT,"updated_at" BIGINT,PRIMARY KEY ("id"));

CREATE INDEX "idx_subscription_pre_consume_records_user_subscription_id" ON "LDW_AI_AGG"."subscription_pre_consume_records"("user_subscription_id");

CREATE INDEX "idx_subscription_pre_consume_records_user_id" ON "LDW_AI_AGG"."subscription_pre_consume_records"("user_id");

CREATE UNIQUE INDEX "idx_subscription_pre_consume_records_request_id" ON "LDW_AI_AGG"."subscription_pre_consume_records"("request_id");

CREATE INDEX "idx_subscription_pre_consume_records_updated_at" ON "LDW_AI_AGG"."subscription_pre_consume_records"("updated_at");

CREATE INDEX "idx_subscription_pre_consume_records_status" ON "LDW_AI_AGG"."subscription_pre_consume_records"("status");

CREATE TABLE "LDW_AI_AGG"."custom_oauth_providers" ("id" BIGINT IDENTITY(1,1),"name" varchar(64) NOT NULL,"slug" varchar(64) NOT NULL,"icon" varchar(128) DEFAULT '',"enabled" BIT DEFAULT 0,"client_id" varchar(256),"client_secret" varchar(512),"authorization_endpoint" varchar(512),"token_endpoint" varchar(512),"user_info_endpoint" varchar(512),"scopes" varchar(256) DEFAULT 'openid profile email',"user_id_field" varchar(128) DEFAULT 'sub',"username_field" varchar(128) DEFAULT 'preferred_username',"display_name_field" varchar(128) DEFAULT 'name',"email_field" varchar(128) DEFAULT 'email',"well_known" varchar(512),"auth_style" BIGINT DEFAULT 0,"access_policy" text,"access_denied_message" varchar(512),"created_at" DATETIME WITH TIME ZONE,"updated_at" DATETIME WITH TIME ZONE,PRIMARY KEY ("id"));

CREATE UNIQUE INDEX "idx_custom_oauth_providers_slug" ON "LDW_AI_AGG"."custom_oauth_providers"("slug");

CREATE TABLE "LDW_AI_AGG"."user_oauth_bindings" ("id" BIGINT IDENTITY(1,1),"user_id" BIGINT NOT NULL,"provider_id" BIGINT NOT NULL,"provider_user_id" varchar(256) NOT NULL,"created_at" DATETIME WITH TIME ZONE,PRIMARY KEY ("id"));

CREATE UNIQUE INDEX "ux_provider_userid" ON "LDW_AI_AGG"."user_oauth_bindings"("provider_id","provider_user_id");

CREATE UNIQUE INDEX "ux_user_provider" ON "LDW_AI_AGG"."user_oauth_bindings"("user_id","provider_id");

CREATE TABLE "LDW_AI_AGG"."perf_metrics" ("id" BIGINT IDENTITY(1,1),"model_name" VARCHAR(128 CHAR),"group" VARCHAR(64 CHAR),"bucket_ts" BIGINT,"request_count" BIGINT DEFAULT 0,"success_count" BIGINT DEFAULT 0,"total_latency_ms" BIGINT DEFAULT 0,"ttft_sum_ms" BIGINT DEFAULT 0,"ttft_count" BIGINT DEFAULT 0,"output_tokens" BIGINT DEFAULT 0,"generation_ms" BIGINT DEFAULT 0,PRIMARY KEY ("id"));

CREATE INDEX "idx_perf_bucket_ts" ON "LDW_AI_AGG"."perf_metrics"("bucket_ts");

CREATE UNIQUE INDEX "idx_perf_model_group_bucket" ON "LDW_AI_AGG"."perf_metrics"("model_name","group","bucket_ts");

CREATE TABLE "LDW_AI_AGG"."system_instances" ("node_name" varchar(128),"info" text,"started_at" BIGINT,"last_seen_at" BIGINT,"created_at" BIGINT,"updated_at" BIGINT,PRIMARY KEY ("node_name"));

CREATE INDEX "idx_system_instances_updated_at" ON "LDW_AI_AGG"."system_instances"("updated_at");

CREATE INDEX "idx_system_instances_created_at" ON "LDW_AI_AGG"."system_instances"("created_at");

CREATE INDEX "idx_system_instances_last_seen_at" ON "LDW_AI_AGG"."system_instances"("last_seen_at");

CREATE INDEX "idx_system_instances_started_at" ON "LDW_AI_AGG"."system_instances"("started_at");

CREATE TABLE "LDW_AI_AGG"."system_tasks" ("id" BIGINT IDENTITY(1,1),"task_id" varchar(64),"type" varchar(64),"status" varchar(32),"active_key" varchar(64),"payload" text,"state" text,"result" text,"error" text,"locked_by" varchar(128),"created_at" BIGINT,"updated_at" BIGINT,PRIMARY KEY ("id"));

CREATE INDEX "idx_system_tasks_status" ON "LDW_AI_AGG"."system_tasks"("status");

CREATE INDEX "idx_system_tasks_type" ON "LDW_AI_AGG"."system_tasks"("type");

CREATE UNIQUE INDEX "idx_system_tasks_task_id" ON "LDW_AI_AGG"."system_tasks"("task_id");

CREATE INDEX "idx_system_tasks_updated_at" ON "LDW_AI_AGG"."system_tasks"("updated_at");

CREATE INDEX "idx_system_tasks_created_at" ON "LDW_AI_AGG"."system_tasks"("created_at");

CREATE INDEX "idx_system_tasks_locked_by" ON "LDW_AI_AGG"."system_tasks"("locked_by");

CREATE UNIQUE INDEX "idx_system_tasks_active_key" ON "LDW_AI_AGG"."system_tasks"("active_key");

CREATE TABLE "LDW_AI_AGG"."system_task_locks" ("type" varchar(64),"task_id" varchar(64),"locked_by" varchar(128),"locked_until" BIGINT,"updated_at" BIGINT,PRIMARY KEY ("type"));

CREATE INDEX "idx_system_task_locks_updated_at" ON "LDW_AI_AGG"."system_task_locks"("updated_at");

CREATE INDEX "idx_system_task_locks_locked_until" ON "LDW_AI_AGG"."system_task_locks"("locked_until");

CREATE INDEX "idx_system_task_locks_locked_by" ON "LDW_AI_AGG"."system_task_locks"("locked_by");

CREATE INDEX "idx_system_task_locks_task_id" ON "LDW_AI_AGG"."system_task_locks"("task_id");

CREATE TABLE "LDW_AI_AGG"."casbin_rule" ("id" BIGINT IDENTITY(1,1),"ptype" VARCHAR(100 CHAR),"v0" VARCHAR(100 CHAR),"v1" VARCHAR(100 CHAR),"v2" VARCHAR(100 CHAR),"v3" VARCHAR(100 CHAR),"v4" VARCHAR(100 CHAR),"v5" VARCHAR(100 CHAR),PRIMARY KEY ("id"));

CREATE UNIQUE INDEX "idx_casbin_rule_unique" ON "LDW_AI_AGG"."casbin_rule"("ptype","v0","v1","v2","v3","v4","v5");

CREATE INDEX "idx_casbin_rule" ON "LDW_AI_AGG"."casbin_rule"("ptype","v0","v1","v2","v3","v4","v5");

CREATE TABLE "LDW_AI_AGG"."authz_roles" ("id" BIGINT IDENTITY(1,1),"key" VARCHAR(64 CHAR) NOT NULL,"name" VARCHAR(100 CHAR) NOT NULL,"description" text,"built_in" BIT,"enabled" BIT,"sort" BIGINT,"created_at" BIGINT,"updated_at" BIGINT,PRIMARY KEY ("id"));

CREATE UNIQUE INDEX "idx_authz_roles_key" ON "LDW_AI_AGG"."authz_roles"("key");

CREATE TABLE "LDW_AI_AGG"."dm8_transaction_locks" ("name" VARCHAR(64 CHAR),PRIMARY KEY ("name"));

INSERT INTO "LDW_AI_AGG"."dm8_transaction_locks" ("name") VALUES ('normalized_email');

CREATE UNIQUE INDEX "uk_prefill_name" ON "LDW_AI_AGG"."prefill_groups" (CASE WHEN "deleted_at" IS NULL THEN "name" ELSE NULL END);

CREATE TABLE "LDW_AI_AGG"."subscription_plans" ("id" BIGINT IDENTITY(1,1),"title" varchar(128) NOT NULL,"subtitle" varchar(255) DEFAULT '',"price_amount" decimal(10,6) NOT NULL DEFAULT 0,"currency" varchar(8) NOT NULL DEFAULT 'USD',"duration_unit" varchar(16) NOT NULL DEFAULT 'month',"duration_value" BIGINT NOT NULL DEFAULT 1,"custom_seconds" bigint NOT NULL DEFAULT 0,"enabled" BIT DEFAULT 1,"sort_order" BIGINT DEFAULT 0,"allow_balance_pay" BIT,"allow_wallet_overflow" BIT,"stripe_price_id" varchar(128) DEFAULT '',"creem_product_id" varchar(128) DEFAULT '',"waffo_pancake_product_id" varchar(128) DEFAULT '',"max_purchase_per_user" BIGINT DEFAULT 0,"upgrade_group" varchar(64) DEFAULT '',"downgrade_group" varchar(64) DEFAULT '',"total_amount" bigint NOT NULL DEFAULT 0,"quota_reset_period" varchar(16) DEFAULT 'never',"quota_reset_custom_seconds" bigint DEFAULT 0,"created_at" BIGINT,"updated_at" BIGINT,PRIMARY KEY ("id"));

-- BEGIN LOG SCHEMA

CREATE TABLE "LDW_AI_AGG"."logs" ("id" BIGINT IDENTITY(1,1),"user_id" BIGINT,"created_at" BIGINT,"type" BIGINT,"content" VARCHAR(8188 CHAR),"username" VARCHAR(255 CHAR) DEFAULT '',"token_name" VARCHAR(255 CHAR) DEFAULT '',"model_name" VARCHAR(255 CHAR) DEFAULT '',"quota" BIGINT DEFAULT 0,"prompt_tokens" BIGINT DEFAULT 0,"completion_tokens" BIGINT DEFAULT 0,"use_time" BIGINT DEFAULT 0,"is_stream" BIT,"channel_id" BIGINT,"channel_name" VARCHAR(8188 CHAR),"token_id" BIGINT DEFAULT 0,"group" VARCHAR(255 CHAR),"ip" VARCHAR(255 CHAR) DEFAULT '',"request_id" varchar(64) DEFAULT '',"upstream_request_id" varchar(128) DEFAULT '',"other" VARCHAR(8188 CHAR),PRIMARY KEY ("id"));

CREATE INDEX "idx_logs_token_name" ON "LDW_AI_AGG"."logs"("token_name");

CREATE INDEX "index_username_model_name" ON "LDW_AI_AGG"."logs"("model_name","username");

CREATE INDEX "idx_logs_username" ON "LDW_AI_AGG"."logs"("username");

CREATE INDEX "idx_created_at_type" ON "LDW_AI_AGG"."logs"("created_at","type");

CREATE INDEX "idx_logs_user_id" ON "LDW_AI_AGG"."logs"("user_id");

CREATE INDEX "idx_user_id_id" ON "LDW_AI_AGG"."logs"("user_id","id");

CREATE INDEX "idx_created_at_id" ON "LDW_AI_AGG"."logs"("created_at","id");

CREATE INDEX "idx_logs_upstream_request_id" ON "LDW_AI_AGG"."logs"("upstream_request_id");

CREATE INDEX "idx_logs_request_id" ON "LDW_AI_AGG"."logs"("request_id");

CREATE INDEX "idx_logs_ip" ON "LDW_AI_AGG"."logs"("ip");

CREATE INDEX "idx_logs_group" ON "LDW_AI_AGG"."logs"("group");

CREATE INDEX "idx_logs_token_id" ON "LDW_AI_AGG"."logs"("token_id");

CREATE INDEX "idx_logs_channel_id" ON "LDW_AI_AGG"."logs"("channel_id");

CREATE INDEX "idx_logs_model_name" ON "LDW_AI_AGG"."logs"("model_name");

CREATE TABLE "LDW_AI_AGG"."audit_logs" ("id" BIGINT IDENTITY(1,1),"event_id" varchar(64),"user_id" BIGINT,"username" varchar(64),"actor_role" BIGINT,"created_at" bigint,"category" varchar(24),"action" varchar(128),"token_ref" varchar(64),"auth_method" varchar(24),"ip" varchar(64),"user_agent" varchar(512),"method" varchar(16),"route" varchar(255),"status" BIGINT,"success" BIT,"request_id" varchar(64),"content" text,"other" CLOB,PRIMARY KEY ("id"));

CREATE UNIQUE INDEX "idx_audit_logs_event_id" ON "LDW_AI_AGG"."audit_logs"("event_id");

CREATE INDEX "idx_audit_logs_request_id" ON "LDW_AI_AGG"."audit_logs"("request_id");

CREATE INDEX "idx_audit_logs_category" ON "LDW_AI_AGG"."audit_logs"("category");

CREATE INDEX "idx_audit_logs_created_at" ON "LDW_AI_AGG"."audit_logs"("created_at");

CREATE INDEX "idx_audit_token_time" ON "LDW_AI_AGG"."audit_logs"("token_ref","created_at");

CREATE INDEX "idx_audit_logs_username" ON "LDW_AI_AGG"."audit_logs"("username");

CREATE INDEX "idx_audit_user_time" ON "LDW_AI_AGG"."audit_logs"("user_id","created_at");

-- END LOG SCHEMA

-- 达梦 DM8 全新空库初始化；仅执行一次，不用于升级已有数据库。
-- 固定目标 schema 为 LDW_AI_AGG；所有建表、索引目标和固定记录插入均显式指定该 schema。
-- 要求实例 CASE_SENSITIVE=1；先创建 LDW_AI_AGG 用户及其同名 schema，再用具备建表/建索引权限的账号执行。
-- 生产主库与日志库共用 LDW_AI_AGG：执行整个文件。启动应用前完成 SQL。
-- 若以后改用独立日志 schema，执行日志段前须将该段的 LDW_AI_AGG 明确替换为目标日志 schema。
-- 运行账号需要正常业务表的读写权限，无需为启动迁移授予建改表或 SELECT ANY DICTIONARY 权限。
