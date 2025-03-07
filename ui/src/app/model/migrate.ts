import { IDataflowConfig } from "./profile";
import ITargetDetails from "./target-details";

export default interface IMigrationDetails {
    TargetDetails: ITargetDetails
    DataflowConfig: IDataflowConfig
    MigrationType: string
    MigrationMode: string
}

export interface IProgress {
    Progress: number
    ErrorMessage: string
    ProgressStatus: number
}

export interface IGeneratedResources {
    DatabaseName: string
    DatabaseUrl: string
    BucketName: string
    BucketUrl: string
    DataStreamJobName: string
    DataStreamJobUrl: string
    DataflowJobName: string
    DataflowJobUrl: string
}

export interface ISourceAndTargetDetails {
    SpannerDatabaseName: string
    SpannerDatabaseUrl: string
    SourceDatabaseName: string
    SourceDatabaseType: string
}