use sea_orm::entity::prelude::*;

#[derive(Clone, Debug, PartialEq, DeriveEntityModel)]
#[sea_orm(table_name = "sync_progress")]
pub struct Model {
    #[sea_orm(primary_key)]
    pub id: i64,
    pub sync_history_id: i64,
    pub total: i32,
    pub downloaded: i32,
    pub failed: i32,
    #[sea_orm(column_type = "String(StringLen::N(255))", nullable)]
    pub current_file: Option<String>,
    pub updated_at: DateTimeWithTimeZone,
}

#[derive(Copy, Clone, Debug, EnumIter, DeriveRelation)]
pub enum Relation {
    #[sea_orm(
        belongs_to = "super::sync_history::Entity",
        from = "Column::SyncHistoryId",
        to = "super::sync_history::Column::Id"
    )]
    SyncHistory,
}

impl Related<super::sync_history::Entity> for Entity {
    fn to() -> RelationDef {
        Relation::SyncHistory.def()
    }
}

impl ActiveModelBehavior for ActiveModel {}
