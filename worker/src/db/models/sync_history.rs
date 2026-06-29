use sea_orm::entity::prelude::*;

#[derive(Clone, Debug, PartialEq, DeriveEntityModel)]
#[sea_orm(table_name = "sync_history")]
pub struct Model {
    #[sea_orm(primary_key)]
    pub id: i64,
    pub started_at: DateTimeWithTimeZone,
    pub completed_at: Option<DateTimeWithTimeZone>,
    #[sea_orm(column_name = "previous_commit", nullable)]
    pub previous_commit: Option<String>,
    #[sea_orm(column_name = "target_commit")]
    pub target_commit: String,
    pub status: String, // running, success, failed
    pub added_count: i32,
    pub updated_count: i32,
    pub deleted_count: i32,
    pub error_message: Option<String>,
    pub triggered_by: String,
    pub created_at: DateTimeWithTimeZone,
}

#[derive(Copy, Clone, Debug, EnumIter, DeriveRelation)]
pub enum Relation {
    #[sea_orm(has_one = "super::sync_progress::Entity")]
    SyncProgress,
}

impl Related<super::sync_progress::Entity> for Entity {
    fn to() -> RelationDef {
        Relation::SyncProgress.def()
    }
}

impl ActiveModelBehavior for ActiveModel {}
